// Draws the MCP view in Chromium the two ways hosts show it, and checks
// what a person would see (see views_browser_test.go):
//
//   - openai: the OpenAI Apps SDK's way. window.openai is set before the
//     page's scripts run, toolOutput and toolResponseMetadata null at
//     first and then set by an openai:set_globals event, as ChatGPT does
//     when the result arrives;
//     notifyIntrinsicHeight resizes the frame.
//   - mcp-apps: the MCP Apps way. The host answers ui/initialize with its
//     context, sends ui/notifications/tool-input and tool-result once the
//     view says it's initialized, and sizes the frame as
//     ui/notifications/size-changed says.
//
// Usage: node view.mjs cases.json [screenshot folder]. It exits 0 when
// every check passes, 3 when there's no browser to run, 1 otherwise.
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { join } from "node:path";

const require = createRequire(import.meta.url);
const { chromium } = require("playwright");
const [, , casesFile, shots] = process.argv;
const cases = JSON.parse(readFileSync(casesFile, "utf8"));
const failures = [];
const check = (ok, what) => { if (!ok) failures.push(what); };

let browser;
try {
  browser = await chromium.launch();
} catch (e) {
  console.error(String(e).split("\n")[0]);
  process.exit(3);
}

// js is v as a script's literal, safe inside a script element.
const js = (v) => JSON.stringify(v).replace(/</g, "\\u003c");

// openaiDoc is the view's page with window.openai set first.
function openaiDoc(page, args, theme) {
  const boot = `<script>
window.openai={theme:${js(theme)},displayMode:"inline",maxHeight:520,locale:"en-US",
  toolInput:${js(args)},toolOutput:null,toolResponseMetadata:null,widgetState:null,
  setWidgetState:function(){return Promise.resolve()},callTool:function(){return Promise.reject(new Error("no tools here"))},
  notifyIntrinsicHeight:function(h){parent.postMessage({harness:"height",height:h},"*")}};
</script>`;
  return page.replace("<head>", "<head>" + boot);
}

// hostPage is the page around the frame: it relays the frame's size and,
// as an MCP Apps host, speaks the extension's messages.
function hostPage(mode, doc, c, theme) {
  const hc = { theme, displayMode: "inline", containerDimensions: { maxHeight: 520 }, locale: "en-US", platform: "web" };
  return `<!doctype html><html><body style="margin:24px;background:${theme === "dark" ? "#111" : "#eee"}">
<iframe id="f" sandbox="allow-scripts" style="width:720px;height:150px;border:1px solid #888;display:block"></iframe>
<script>
var f=document.getElementById("f"),seen=[];window.seen=seen;
window.addEventListener("message",function(e){
  if(e.source!==f.contentWindow)return;var m=e.data;seen.push(m);
  if(m.harness==="height"){f.style.height=m.height+"px";return}
  if(${js(mode)}!=="mcp-apps"||!m||m.jsonrpc!=="2.0")return;
  function post(x){f.contentWindow.postMessage(Object.assign({jsonrpc:"2.0"},x),"*")}
  if(m.method==="ui/initialize")post({id:m.id,result:{protocolVersion:"2026-01-26",hostInfo:{name:"harness",version:"1"},hostCapabilities:{},hostContext:${js(hc)}}});
  if(m.method==="ui/notifications/initialized"){post({method:"ui/notifications/tool-input",params:{arguments:${js(c.args)}}});
    post({method:"ui/notifications/tool-result",params:${js(c.result)}})}
  if(m.method==="ui/notifications/size-changed")f.style.height=m.params.height+"px";
});
f.srcdoc=${js(doc)};
</script></body></html>`;
}

async function run(mode, name, c, theme) {
  const page = await browser.newPage({ viewport: { width: 800, height: 700 } });
  const errors = [];
  page.on("pageerror", (e) => errors.push(String(e)));
  const doc = mode === "openai" ? openaiDoc(cases.page, c.args, theme) : cases.page;
  await page.setContent(hostPage(mode, doc, c, theme));
  const frame = page.frameLocator("#f");
  if (mode === "openai") {
    await frame.locator("#o12-view").waitFor({ state: "attached" });
    await page.waitForTimeout(100);
    await frame.locator("body").evaluate((_, r) => {
      window.openai.toolOutput = r.structuredContent;
      window.openai.toolResponseMetadata = r._meta;
      const globals = { toolOutput: r.structuredContent, toolResponseMetadata: r._meta };
      window.dispatchEvent(new CustomEvent("openai:set_globals", { detail: { globals } }));
    }, c.result);
  }
  const what = `${mode} ${name}`;
  try {
    await frame.locator("#o12-view > *").first().waitFor({ timeout: 5000 });
  } catch {
    failures.push(`${what}: nothing drawn`);
    await page.close();
    return;
  }
  await page.waitForTimeout(300);
  const v = c.result._meta["o12/view"];
  const state = () => frame.locator("body").evaluate(() => {
    const sh = document.querySelector("#o12-view .sheet");
    return {
      book: document.querySelector(".panel .book").textContent,
      name: document.getElementById("o12-name").textContent,
      input: document.getElementById("o12-input").textContent,
      where: document.getElementById("o12-context").textContent,
      theme: document.documentElement.className,
      height: Math.ceil(document.documentElement.getBoundingClientRect().height),
      scroll: sh ? { client: sh.clientHeight, full: sh.scrollHeight } : null,
    };
  });
  let s = await state();
  const frameHeight = await page.locator("#f").evaluate((f) => f.getBoundingClientRect().height);
  check(s.book === v.title && s.where === v.where, `${what}: panel says ${s.book} / ${s.where}`);
  check(s.theme === theme, `${what}: theme ${s.theme}, want ${theme}`);
  check(Math.abs(frameHeight - s.height - 2) <= 2, `${what}: frame ${frameHeight}px for ${s.height}px of view`);
  check(s.height <= 520, `${what}: ${s.height}px, more than the host's room`);
  check(errors.length === 0, `${what}: script errors ${errors}`);
  if (c.grid) {
    check(s.name === "A1" && s.input === c.grid.a1, `${what}: pointer on ${s.name} showing ${s.input}`);
    await frame.locator(`td[data-a="${c.grid.cell}"]`).click();
    s = await state();
    check(s.name === c.grid.cell && s.input === c.grid.input, `${what}: clicking ${c.grid.cell} shows ${s.name} ${s.input}`);
    await page.keyboard.press("ArrowDown");
    s = await state();
    check(s.name === c.grid.below, `${what}: the arrow key moved to ${s.name}`);
    check(s.scroll && s.scroll.full > s.scroll.client, `${what}: the grid doesn't scroll (${JSON.stringify(s.scroll)})`);
    const header = await frame.locator("#o12-view .sheet").evaluate((sh) => {
      sh.scrollTop = 400;
      const th = sh.querySelector("thead th"), top = sh.getBoundingClientRect().top;
      return { scrolled: sh.scrollTop, headerAt: Math.round(th.getBoundingClientRect().top - top) };
    });
    check(header.scrolled > 0 && header.headerAt === 0, `${what}: scrolled ${header.scrolled}, column header ${header.headerAt}px down`);
    await frame.locator("#o12-view .sheet").evaluate((sh) => { sh.scrollTop = 0; });
    const style = await frame.locator(`td[data-a="${c.grid.cell}"]`).evaluate((td) => getComputedStyle(td).textAlign);
    check(style === "right", `${what}: a number is aligned ${style}`);
  } else {
    check(await frame.locator("#o12-view svg").count() === 1, `${what}: no chart drawn`);
    check(s.name === v.name, `${what}: name box ${s.name}`);
  }
  if (mode === "mcp-apps") {
    const seen = await page.evaluate(() => window.seen.map((m) => m.method || (m.id ? "response" : m.harness)));
    check(seen[0] === "ui/initialize" && seen.includes("ui/notifications/initialized") && seen.includes("ui/notifications/size-changed"),
      `${what}: messages ${seen}`);
  }
  if (shots) {
    await page.screenshot({ path: join(shots, `${mode}-${name}-${theme}.png`) });
  }
  await page.close();
}

for (const mode of ["openai", "mcp-apps"]) {
  for (const [name, c] of Object.entries(cases.cases)) {
    for (const theme of ["light", "dark"]) {
      await run(mode, name, c, theme);
    }
  }
}
await browser.close();
if (failures.length) {
  console.error(failures.join("\n"));
  process.exit(1);
}
