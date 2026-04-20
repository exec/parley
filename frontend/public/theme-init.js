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

  // ── Viewport height tracking ─────────────────────────────────────────────
  // Track visualViewport.height so the app shrinks above the iOS keyboard
  // (keyboard doesn't affect 100svh/100dvh — Apple treats it as an overlay).
  // Without this, iOS auto-scrolls the whole WKWebView on input focus and
  // the chat header slides off screen.
  function updateAppHeight(){
    var h = window.visualViewport ? window.visualViewport.height : window.innerHeight;
    document.documentElement.style.setProperty('--app-height', h + 'px');
    // Cancel any document-level scroll that iOS may have initiated to push
    // the input into view. Our inner containers handle their own scrolling.
    if (window.scrollY !== 0 || window.scrollX !== 0) window.scrollTo(0, 0);
  }
  updateAppHeight();
  if (window.visualViewport) {
    window.visualViewport.addEventListener('resize', updateAppHeight);
    window.visualViewport.addEventListener('scroll', updateAppHeight);
  }
  window.addEventListener('orientationchange', function(){
    setTimeout(updateAppHeight, 150);
  });
})();
