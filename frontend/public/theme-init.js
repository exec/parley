(function(){
  var t=localStorage.getItem('parley-theme')||'abyss';
  if(t==='custom'){
    var b=localStorage.getItem('parley-theme-base')||'abyss';
    document.body.dataset.theme=b;
    var c=localStorage.getItem('parley-custom-css');
    if(c){var s=document.createElement('style');s.id='custom-theme';s.textContent=c;document.head.appendChild(s);}
  } else {
    document.body.dataset.theme=t;
  }
})();
