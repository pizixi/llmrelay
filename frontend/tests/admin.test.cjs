const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { test } = require("node:test");

const admin = vm.createContext({
  URL,
  localStorage: { getItem: () => null },
  document: { readyState: "loading", addEventListener() {} },
  setInterval() {},
});
vm.runInContext(fs.readFileSync(path.join(__dirname, "../assets/admin.js"), "utf8"), admin);
const parse = (...args) => admin.parseBulkUpstreams(...args);

test("批量上游识别逐行分隔符并保留 URL 路径和 Key", () => {
  const result = parse("# 上游列表\r\n\r\nhttps://api.example.com/v1 | sk-one | 主上游\r\napi.other.com/v1,sk-two\nhttps://third.example.com\tsk-three\nhttps://fourth.example.com sk-four\nhttps://fifth.example.com，sk-five", "", "anthropic");
  assert.equal(result.errors.length, 0);
  assert.equal(result.rows.length, 5);
  assert.equal(result.rows[0].name, "主上游");
  assert.equal(result.rows[0].up.api_key, "sk-one");
  assert.equal(result.rows[1].up.base_url, "https://api.other.com/v1");
  assert.ok(result.rows.every(({ up }) => up.api_type === "anthropic" && up.bridge_mode === "compatible"));
});

test("自定义分隔符支持 Key 中包含其他分隔符", () => {
  const result = parse("https://api.example.com/v1::secret,with|symbols::自定义", "::", "openai-responses");
  assert.equal(result.errors.length, 0);
  assert.equal(result.rows[0].up.api_key, "secret,with|symbols");
  assert.equal(result.rows[0].name, "自定义");
});

test("重复判断包含 URL、Key、协议，并归一化域名和末尾斜线", () => {
  const existing = [{ base_url: "https://API.example.com/v1/", api_key: "sk-one", api_type: "openai" }];
  const result = parse("https://api.example.com/v1 | sk-one\nhttps://api.example.com/v1 | sk-two\nhttps://api.example.com/v1/ | sk-two", "", "openai", existing);
  assert.equal(result.duplicates, 2);
  assert.equal(result.rows.length, 1);
  assert.equal(parse("https://api.example.com/v1 | sk-one", "", "anthropic", existing).rows.length, 1);
});

test("自动命名保留已有名称并对重名添加后缀", () => {
  const names = new Set(["主上游", "主上游-2"]);
  const result = parse("https://a.example.com | sk-a | 主上游\nhttps://b.example.com | sk-b | 主上游", "", "openai", [], names);
  assert.equal(result.rows[0].name, "主上游-3");
  assert.equal(result.rows[1].name, "主上游-4");
  assert.equal(names.size, 2);
});

test("无效行提示真实行号且不暴露 Key", () => {
  const secret = "sk-sensitive-test-value";
  const result = parse("# 注释\n\nftp://invalid.example | " + secret + "\nhttps://api.example.com | \nhttps://api.example.com | secret with spaces\nhttps://api.example.com | sk-ok", "", "openai");
  assert.equal(result.errors.length, 3);
  assert.match(result.errors[0], /第 3 行/);
  assert.ok(!result.errors.join(" ").includes(secret));
  assert.equal(result.rows.length, 1);
});

test("超出批量大小时拒绝导入", () => {
  const result = parse(Array.from({ length: 501 }, (_, i) => `https://api.example.com | sk-${i}`).join("\n"), "", "openai");
  assert.equal(result.rows.length, 0);
  assert.match(result.errors[0], /500/);
});

test("相同别名和上游的不同实际模型保留为独立统计行", () => {
  const rows = [
    { model: "chat", upstream: "primary", upstream_model: "model-a", date: "2026-11-04", total_tokens: 20 },
    { model: "chat", upstream: "primary", upstream_model: "model-b", date: "2026-11-04", total_tokens: 30 },
    { model: "other-alias", upstream: "primary", upstream_model: "model-a", date: "2026-11-04", total_tokens: 10 },
  ];
  assert.equal(Object.keys(admin.statsRowMap(rows)).length, 3);
  assert.equal(Object.keys(admin.statsModelUpstreamDayMap(rows)).length, 3);
  assert.equal(Object.keys(admin.statsModelUpstreamDayMap(rows, "2026-11-03")).length, 0);
  assert.equal(Object.values(admin.statsModelUpstreamDayMap(rows)).reduce((sum, row) => sum + row.total_tokens, 0), 60);
});

test("旧统计数据缺少实际模型时仍保留请求模型", () => {
  const rows = [{ model: "legacy", upstream: "历史数据", date: "2026-11-01" }];
  assert.equal(Object.keys(admin.statsRowMap(rows)).length, 1);
  assert.equal(Object.keys(admin.statsModelUpstreamDayMap(rows)).length, 1);
});

test("模型对话按顺序携带完整的历史问答，并保留原始消息", () => {
  const history = [{ role: "user", content: "记住项目叫星河" }, { role: "assistant", content: "已记住星河项目。" }];
  const messages = admin.buildModelTestMessages(history, " 项目叫什么？ ");
  assert.deepEqual(JSON.parse(JSON.stringify(messages)), history.concat({ role: "user", content: "项目叫什么？" }));
  assert.equal(history.length, 2);
});

test("上下文最多携带最近 20 轮，超出字符预算时整轮移除", () => {
  const history = Array.from({ length: 50 }, (_, i) => ({ role: i % 2 ? "assistant" : "user", content: String(i) }));
  const messages = admin.buildModelTestMessages(history, "继续");
  assert.equal(messages.length, 41);
  assert.equal(messages[0].content, "10");
  const largeHistory = [{ role: "user", content: "旧问题" }, { role: "assistant", content: "旧回答" },
    { role: "user", content: "新问题" }, { role: "assistant", content: "😀".repeat(32760) }];
  const bounded = admin.buildModelTestMessages(largeHistory, "接着说");
  assert.equal(bounded.length, 3);
  assert.equal(bounded[0].content, "新问题");
  assert.equal(admin.buildModelTestMessages(largeHistory, "大".repeat(32768)).length, 1);
});

test("模型流保留用量，并分别提取正文和思考内容", () => {
  const event = admin.modelTestStreamEvent('data: {"choices":[{"delta":{"content":[{"type":"text","text":"回答"},{"type":"thinking","text":"分析"}]}}],"usage":{"total_tokens":42}}');
  assert.equal(event.text, "回答");
  assert.equal(event.reasoning, "分析");
  assert.equal(event.usage.total_tokens, 42);
  assert.equal(admin.modelTestStreamEvent("data: [DONE]").done, true);
});
