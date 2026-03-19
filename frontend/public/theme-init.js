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

  // ── Stable viewport height ───────────────────────────────────────────────
  // 100dvh resizes whenever the iOS URL bar slides in/out (on scroll),
  // causing layout shifts between views. Instead we capture the height once
  // and only update on orientation change so the layout stays rock-solid.
  function setAppHeight(){
    var h=window.visualViewport?window.visualViewport.height:window.innerHeight;
    document.documentElement.style.setProperty('--app-height',h+'px');
  }
  setAppHeight();
  window.addEventListener('orientationchange',function(){setTimeout(setAppHeight,100);});
  // Also update on visualViewport resize but only for large changes (keyboard),
  // not small scroll-triggered URL bar slides (< 100px).
  if(window.visualViewport){
    var _lastH=window.visualViewport.height;
    window.visualViewport.addEventListener('resize',function(){
      var h=window.visualViewport.height;
      if(Math.abs(h-_lastH)>80){_lastH=h;setAppHeight();}
    });
  }
})();
