(function(){
  var t=localStorage.getItem('theme');
  if(t==='dark'){document.documentElement.setAttribute('data-theme','dark')}
  document.addEventListener('DOMContentLoaded',function(){
    updateThemeIcon(localStorage.getItem('theme')||'light');
    var form=document.querySelector('form[action="/login"]');
    var btn=form&&form.querySelector('.btn');
    if(form&&btn){
      form.addEventListener('submit',function(){
        if(btn.disabled)return;
        btn.disabled=true;
        btn.dataset.originalText=btn.textContent;
        btn.textContent='登录中…';
      });
    }
    var params=new URLSearchParams(window.location.search);
    if(params.get('error')||params.get('failed')){
      var msg=document.getElementById('login-msg');
      if(msg){
        msg.style.display='block';
        msg.textContent=params.get('error')||'密码错误，请重试';
      }
    }
  });
})();
function getThemeIcon(theme){if(theme==='dark'){return '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>'}return '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>'}
function updateThemeIcon(theme){var btn=document.querySelector('.theme-toggle');if(btn)btn.innerHTML=getThemeIcon(theme)}
function toggleTheme(){var d=document.documentElement;var n=d.getAttribute('data-theme')==='dark'?'light':'dark';if(n==='dark')d.setAttribute('data-theme','dark');else d.removeAttribute('data-theme');localStorage.setItem('theme',n);updateThemeIcon(n)}
