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

  // ── Standalone PWA height ────────────────────────────────────────────────
  // The CSS display-mode:standalone media query handles the svh→dvh switch,
  // but during login→app transition React may remount before the cascade
  // resolves. Set an explicit --app-height so there's never a flash.
  // In standalone: use screen dimensions (true full screen).
  // In browser: use innerHeight (current visible area, stable enough on mount).
  var isStandalone = window.navigator.standalone === true ||
    window.matchMedia('(display-mode: standalone)').matches;
  var h = isStandalone
    ? (window.screen.height)  // full physical screen in CSS px
    : (window.visualViewport ? window.visualViewport.height : window.innerHeight);
  document.documentElement.style.setProperty('--app-height', h + 'px');
  window.addEventListener('orientationchange', function(){
    setTimeout(function(){
      var isStandalone2 = window.navigator.standalone === true ||
        window.matchMedia('(display-mode: standalone)').matches;
      var h2 = isStandalone2 ? window.screen.height
        : (window.visualViewport ? window.visualViewport.height : window.innerHeight);
      document.documentElement.style.setProperty('--app-height', h2 + 'px');
    }, 150);
  });
})();
