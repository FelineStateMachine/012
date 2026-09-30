package mcp

// viewScript is the view's side of both hosts' protocols. With
// window.openai (the Apps SDK) it draws toolResponseMetadata's view, takes the
// theme, maxHeight and displayMode from it and from openai:set_globals
// events, and reports its height with notifyIntrinsicHeight. Otherwise
// it speaks MCP Apps over postMessage: ui/initialize with its info and
// display modes, then ui/notifications/initialized; it draws the
// view in the _meta of ui/notifications/tool-result, follows the
// hostContext's theme, containerDimensions and displayMode as they
// change, answers ping and ui/resource-teardown, and sends
// ui/notifications/size-changed as its content's size changes,
// measured as the extension's SDK measures it.
//
// The grid scrolls inside the room it's given: inline, at most 480
// pixels, less when the host's maximum height less the panel's is
// smaller, so a view in a conversation stays a glance; the whole frame
// when the host fixes its height or shows it full screen. A smaller
// range takes only its own height.
const viewScript = `(function(){
var doc=document.documentElement,view=document.getElementById("o12-view"),oai=window.openai;
var room=0,fill=false,shown=null,next=1,lastW=0,lastH=0,sizing=false;
function theme(t){doc.classList.remove("dark","light");if(t==="dark"||t==="light")doc.classList.add(t)}
function send(m){window.parent.postMessage(Object.assign({jsonrpc:"2.0"},m),"*")}
function fit(){
  var sh=view.querySelector(".sheet"),p=document.querySelector(".panel");if(!sh)return;
  var top=p?p.offsetHeight:0,h=fill?window.innerHeight-top:Math.min(room>0?room-top:480,480);
  sh.style.maxHeight=Math.max(h,84)+"px";
}
function measure(){
  var old=doc.style.height;doc.style.height="max-content";
  var h=Math.ceil(doc.getBoundingClientRect().height);doc.style.height=old;return h;
}
function size(){
  if(sizing)return;sizing=true;
  requestAnimationFrame(function(){
    sizing=false;var w=Math.ceil(window.innerWidth),h=measure();
    if(w===lastW&&h===lastH)return;lastW=w;lastH=h;
    if(oai){if(typeof oai.notifyIntrinsicHeight==="function")oai.notifyIntrinsicHeight(h)}
    else send({method:"ui/notifications/size-changed",params:{width:w,height:h}});
  });
}
function show(v){
  if(!v||typeof v.html!=="string"||v===shown)return;
  shown=v;view.innerHTML=v.html;
  var b=document.querySelector(".panel .book"),c=document.getElementById("o12-context"),n=document.getElementById("o12-name");
  if(b)b.textContent=v.title||"";if(c)c.textContent=v.where||"";if(n)n.textContent=v.name||"";
  var first=view.querySelector("td[data-a]");if(first&&window.o12point)window.o12point(first);
  fit();size();
}
function viewOf(meta){return meta&&meta["o12/view"]}
function context(hc){
  if(!hc)return;if(hc.theme)theme(hc.theme);
  var d=hc.containerDimensions;
  if(d){fill=typeof d.height==="number";room=typeof d.maxHeight==="number"?d.maxHeight:0}
  if(hc.displayMode)fill=fill||hc.displayMode==="fullscreen";
  fit();size();
}
function globals(g){
  if(!g)return;if(g.theme)theme(g.theme);
  if(typeof g.maxHeight==="number")room=g.maxHeight;
  if(g.displayMode)fill=g.displayMode==="fullscreen";
  if(g.toolResponseMetadata)show(viewOf(g.toolResponseMetadata));
  fit();size();
}
window.addEventListener("message",function(e){
  var m=e.data;if(!m||m.jsonrpc!=="2.0"||oai)return;
  if(m.id===1&&(m.result||m.error)){if(m.result)context(m.result.hostContext);send({method:"ui/notifications/initialized",params:{}});return}
  switch(m.method){
  case "ui/notifications/tool-result":show(viewOf(m.params&&m.params._meta));break;
  case "ui/notifications/host-context-changed":context(m.params);break;
  case "ui/resource-teardown":case "ping":send({id:m.id,result:{}});break;
  }
});
window.addEventListener("openai:set_globals",function(e){if(e.detail)globals(e.detail.globals)});
window.addEventListener("resize",function(){fit();size()});
new ResizeObserver(size).observe(document.body);
if(oai)globals(oai);
else send({id:next++,method:"ui/initialize",params:{protocolVersion:"2026-01-26",appInfo:{name:"012",version:"1"},appCapabilities:{availableDisplayModes:["inline","fullscreen"]}}});
size();
})();
`
