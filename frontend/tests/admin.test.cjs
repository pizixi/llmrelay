const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { test } = require("node:test");
const adminSource = fs.readFileSync(path.join(__dirname, "../assets/admin.js"), "utf8");

const admin = vm.createContext({
  URL,
  localStorage: { getItem: () => null },
  document: { readyState: "loading", addEventListener() {} },
  setInterval() {},
});
vm.runInContext(adminSource, admin);
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

test("模型流按完整事件解析：支持多行 data 与单换行分隔", () => {
  const parser = admin.createModelTestSSEParser();
  const multiLine = parser.push('data: {"choices":[{"delta":{"content":\ndata: "多行正文"}}]}\n\n', false);
  assert.equal(multiLine.length, 1);
  assert.equal(admin.modelTestStreamEvent(multiLine[0]).text, "多行正文");

  const singleLine = admin.createModelTestSSEParser();
  const events = singleLine.push('data: {"choices":[{"delta":{"content":"一"}}]}\ndata: {"choices":[{"delta":{"content":"二"}}]}\n', false);
  assert.equal(events.length, 2);
  assert.equal(events.map((event) => admin.modelTestStreamEvent(event).text).join(""), "一二");
});

test("模型流保留拒答正文、非流式 message 正文和结束原因", () => {
  const refusal = admin.modelTestStreamEvent('data: {"choices":[{"delta":{"refusal":"无法提供该内容"}}]}');
  assert.equal(refusal.text, "无法提供该内容");

  const full = admin.modelTestNonStreamEvent('{"choices":[{"message":{"content":"完整回答"},"finish_reason":"stop"}]}');
  assert.equal(full.text, "完整回答");
  assert.equal(full.done, true);

  const truncated = admin.modelTestStreamEvent('data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"}}');
  assert.equal(truncated.finishReason, "length");
  assert.equal(admin.modelTestEmptyTextReason("length", true, false).includes("思考阶段"), true);
});

test("模型流兼容顶层 content，并识别非流式错误响应", () => {
  const event = admin.modelTestStreamEvent({ data: '{"content":"顶层正文"}', name: "" });
  assert.equal(event.text, "顶层正文");
  const failed = admin.modelTestNonStreamEvent('{"error":{"message":"bad key"}}');
  assert.equal(failed.error, "bad key");
  const full = admin.modelTestNonStreamEvent('{"content":"非流式正文"}');
  assert.equal(full.text, "非流式正文");
  assert.equal(full.done, true);
});

test("已分类的增量只进入对应的正文或思考通道一次", () => {
  const cases = [
    ["output_text.delta", "回答", ""],
    ["response.output_text.delta", "回答", ""],
    ["reasoning.delta", "", "回答"],
    ["response.reasoning_text.delta", "", "回答"],
    ["refusal.delta", "回答", ""],
    ["response.refusal.delta", "回答", ""],
    ["text_delta", "回答", ""],
  ];
  for (const [type, text, reasoning] of cases) {
    const event = admin.modelTestStreamEvent({ data: JSON.stringify({ type, delta: "回答" }) });
    assert.equal(event.text, text, type);
    assert.equal(event.reasoning, reasoning, type);
  }
});

// 保留真实的请求、流读取和重试流程，仅替换与这些行为无关的页面渲染。
async function runModelTestResponses(bodies, options = {}) {
  const element = () => {
    const children = new Map();
    return {
      dataset: {}, style: {}, isConnected: false,
      scrollTop: 0, scrollHeight: 0, clientHeight: 0,
      querySelector(selector) {
        if (!children.has(selector)) children.set(selector, element());
        return children.get(selector);
      },
      appendChild(child) { (this.added ||= []).push(child); },
    };
  };
  const context = vm.createContext({
    URL, AbortController, DOMException, TextDecoder, Uint8Array, performance,
    localStorage: { getItem: () => null },
    document: { readyState: "loading", addEventListener() {}, createElement: element },
    setInterval() {},
    setTimeout(callback) { callback(); },
  });
  vm.runInContext(adminSource, context);
  for (const name of [
    "resetModelTestThinking", "resizeModelTestInput", "updateModelTestControls",
    "appendModelTestThinking", "flushModelTestOutput", "finishModelTestThinking",
    "updateModelTestContext", "updateModelTestScroll", "setModelTestStatus",
  ]) context[name] = () => {};
  context.buildModelTestTurnHtml = () => "";
  context.saveConfigSilent = async () => {};
  context.appendModelTestOutput = (pop, text) => {
    if (text) pop._modelTestOutputMarkdown += text;
  };
  const requests = [];
  context.apiFetch = async (url, requestOptions) => {
    assert.equal(url, "/api/test-model");
    requests.push(JSON.parse(requestOptions.body));
    return new Response(bodies[Math.min(requests.length - 1, bodies.length - 1)], {
      headers: options.headers || {},
    });
  };
  const pop = element();
  pop._modelTestMode = options.mode || "upstream";
  pop._modelTestUpstream = options.mode === "alias" ? "" : "target";
  pop.querySelector(".model-test-input").value = "你好";
  pop.querySelector(".model-test-model-select").value = options.model || "test-model";
  await context.runModelTest({ closest: () => pop });
  const turn = pop._modelTestActiveTurn;
  return {
    requests,
    state: turn.dataset.state,
    answer: turn._modelTestAnswer,
    history: JSON.parse(JSON.stringify(pop._modelTestMessages || [])),
    errors: (turn.added || []).map((child) => child.textContent),
    metrics: turn.querySelector(".model-test-turn-status").textContent,
    route: pop.querySelector("[data-model-test-route]").textContent,
  };
}

test("有明确结束原因的空回答不重试，保留原始诊断", async () => {
  for (const [finishReason, diagnostic] of [
    ["content_filter", "内容策略"],
    ["length", "输出额度不足"],
    ["tool_calls", "工具调用"],
    ["stop", "模型未返回文本"],
  ]) {
    const body = `data: ${JSON.stringify({ choices: [{ delta: {}, finish_reason: finishReason }] })}\n\ndata: [DONE]\n\n`;
    const result = await runModelTestResponses([body]);
    assert.equal(result.requests.length, 1, finishReason);
    assert.equal(result.state, "error", finishReason);
    assert.ok(result.errors.some((message) => message.includes(diagnostic)), finishReason);
    assert.equal(result.history.length, 0, finishReason);
  }
});

test("空流只重试一次，成功回答加入上下文且不复用首轮用量", async () => {
  const empty = 'data: {"choices":[{"delta":{"role":"assistant"}}],"usage":{"total_tokens":42}}\n\ndata: [DONE]\n\n';
  const answer = 'data: {"choices":[{"delta":{"content":"你好！"},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n';
  const result = await runModelTestResponses([empty, answer]);
  assert.equal(result.requests.length, 2);
  assert.deepEqual(result.requests[0], result.requests[1]);
  assert.equal(result.state, "complete");
  assert.equal(result.answer, "你好！");
  assert.deepEqual(result.history, [{ role: "user", content: "你好" }, { role: "assistant", content: "你好！" }]);
  assert.ok(!result.metrics.includes("Tokens"));

  const stillEmpty = await runModelTestResponses(["data: [DONE]\n\n"]);
  assert.equal(stillEmpty.requests.length, 2);
  assert.equal(stillEmpty.state, "error");
  assert.equal(stillEmpty.history.length, 0);
});

test("完整 JSON 回答进入上下文，HTTP 200 内的上游错误保留且不重试", async () => {
  const success = await runModelTestResponses(['{"choices":[{"message":{"content":"完整回答"},"finish_reason":"stop"}]}']);
  assert.equal(success.requests.length, 1);
  assert.equal(success.state, "complete");
  assert.equal(success.answer, "完整回答");
  assert.equal(success.history[1].content, "完整回答");

  const failure = await runModelTestResponses(['{"error":{"message":"bad test key"}}']);
  assert.equal(failure.requests.length, 1);
  assert.equal(failure.state, "error");
  assert.ok(failure.errors.some((message) => message.includes("bad test key")));
  assert.equal(failure.history.length, 0);
});

test("模型映射测试通过别名请求并展示本次实际路由", async () => {
  const answer = 'data: {"choices":[{"delta":{"content":"你好！"},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n';
  const result = await runModelTestResponses([answer], {
    mode: "alias",
    model: "mapped-model",
    headers: {
      "X-Model-Test-Upstream": "primary",
      "X-Model-Test-Model": "real-model",
    },
  });
  assert.equal(result.requests.length, 1);
  assert.equal(result.requests[0].alias, "mapped-model");
  assert.equal(result.requests[0].upstream, undefined);
  assert.equal(result.route, "→ primary / real-model");
});
