(function(){
  // ── Theme ────────────────────────────────────────────────────────────────
  var colors={'abyss':'#0a1628','rory':'#000000','citron-dark':'#36393f','citron-light':'#ffffff','neon-nights':'#0d0221','sakura':'#fff9fb'};
  function setThemeColor(id){
    var m=document.getElementById('theme-color-meta');
    if(m)m.setAttribute('content',colors[id]||colors['abyss']);
  }
  var t=localStorage.getItem('parley-theme')||'abyss';
  if(t==='custom'){
    var b=localStorage.getItem('parley-theme-base')||'abyss';
    document.body.dataset.theme=b;
    setThemeColor(b);
    var c=localStorage.getItem('parley-custom-css');
    if(c){var s=document.createElement('style');s.id='custom-theme';s.textContent=c;document.head.appendChild(s);}
  } else {
    document.body.dataset.theme=t;
    setThemeColor(t);
  }
})();
