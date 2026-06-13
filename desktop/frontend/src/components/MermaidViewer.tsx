import { useEffect, useState, useCallback } from "react";

/**
 * MermaidViewer — 将 ```mermaid 代码块渲染为内嵌 SVG 图表。
 *
 * 主题适配方案：
 * 1. THEME_COLORS 使用完整主题色（包括 background），mermaid 需要这些来正确着色节点/连线
 * 2. 渲染后用 DOMParser 解析 SVG，仅清除 SVG 画布背景（background-color / 大背景 rect）
 * 3. 节点、连线、文字的颜色由 THEME_COLORS 控制，不清除
 * 4. CSS 容器 var(--bg-elev) 作为最终背景
 */

const THEME_COLORS = {
  dark: {
    primary: "#d97757", primaryText: "#f4f5f7", lineColor: "#c0c4cc",
    background: "#191b22", mainBkg: "#222631", nodeBorder: "#343945",
    clusterBkg: "#111319", clusterBorder: "#252a34", titleColor: "#f4f5f7",
    edgeLabelBackground: "#191b22", textColor: "#f4f5f7", signalColor: "#f4f5f7",
    signalTextColor: "#f4f5f7", labelBoxBkgColor: "#222631", labelBoxBorderColor: "#343945",
    labelTextColor: "#f4f5f7", loopTextColor: "#f4f5f7", noteTextColor: "#f4f5f7",
    actorTextColor: "#f4f5f7", actorBkg: "#222631", actorBorder: "#343945",
    actorLineColor: "#343945", activationBorderColor: "#d97757",
    activationBkgColor: "#222631", sequenceNumberColor: "#f4f5f7",
  },
  light: {
    primary: "#2f5fa8", primaryText: "#1a0f0a", lineColor: "#4b5563",
    background: "#ffffff", mainBkg: "#f2f5f9", nodeBorder: "#d8dee8",
    clusterBkg: "#eef2f7", clusterBorder: "#e7ebf2", titleColor: "#1a1e25",
    edgeLabelBackground: "#ffffff", textColor: "#1a1e25", signalColor: "#1a1e25",
    signalTextColor: "#1a1e25", labelBoxBkgColor: "#f2f5f9", labelBoxBorderColor: "#d8dee8",
    labelTextColor: "#1a1e25", loopTextColor: "#1a1e25", noteTextColor: "#1a1e25",
    actorTextColor: "#1a1e25", actorBkg: "#f2f5f9", actorBorder: "#d8dee8",
    actorLineColor: "#d8dee8", activationBorderColor: "#2f5fa8",
    activationBkgColor: "#f2f5f9", sequenceNumberColor: "#1a1e25",
  },
} as const;

type ThemeName = keyof typeof THEME_COLORS;

function detectTheme(): ThemeName {
  const root = document.documentElement;
  const explicit = root.getAttribute("data-theme");
  if (explicit === "light") return "light";
  if (explicit === "dark") return "dark";
  const bg = getComputedStyle(root).getPropertyValue("--bg").trim();
  if (bg === "#f7f8fb" || bg === "#ffffff") return "light";
  return "dark";
}

/**
 * 用 DOMParser 解析 SVG 字符串，仅清除 SVG 画布背景色。
 * 节点、连线、文字颜色保持不变（由 THEME_COLORS 控制）。
 */
function sanitizeSvgString(raw: string): string {
  try {
    const parser = new DOMParser();
    const doc = parser.parseFromString(raw, "image/svg+xml");
    const svg = doc.querySelector("svg");
    if (!svg) return raw;

    // ① 清除 SVG 根元素 inline style 中的 background-color
    const styleAttr = svg.getAttribute("style") || "";
    const cleanedStyle = styleAttr
      .replace(/background-color:\s*[^;]+;?\s*/gi, "")
      .trim();
    if (cleanedStyle) svg.setAttribute("style", cleanedStyle);
    else svg.removeAttribute("style");

    // ② 修改 <style> 标签中的 SVG 画布背景规则（不影响节点样式）
    doc.querySelectorAll("style").forEach((styleEl) => {
      const text = styleEl.textContent || "";
      // 只替换 SVG 根级背景，保留节点/连线的填充色
      styleEl.textContent = text
        .replace(
          /#mermaid[^\s{]*(?:\s*\.background\s*\{[^}]*\})/g,
          (match) => match.replace(/fill:\s*[^;]+/, "fill: transparent")
        )
        .replace(
          /\.background\s*\{[^}]*\}/g,
          ".background { fill: transparent; }"
        );
    });

    // ③ 清除宽/高 > 500 的背景 rect（整幅背景矩形）
    //    保留小尺寸 rect（节点矩形/文本框等）
    doc.querySelectorAll("rect").forEach((r) => {
      const w = parseFloat(r.getAttribute("width") || "0");
      const h = parseFloat(r.getAttribute("height") || "0");
      const cls = (r.getAttribute("class") || "").toLowerCase();
      // 仅清除明确是背景的矩形
      if (cls.includes("background") || (w > 500 && h > 500)) {
        r.setAttribute("fill", "transparent");
      }
    });

    // ④ Defensive sanitization. securityLevel="strict" already disables click
    //    handlers and HTML labels, but mermaid's SVG can still carry inline
    //    <script> elements or on*= event attributes via theme-variable
    //    interpolation. Strip them — diagrams are display-only.
    doc.querySelectorAll("script").forEach((s) => s.remove());
    doc.querySelectorAll("*").forEach((el) => {
      for (const attr of Array.from(el.attributes)) {
        const name = attr.name.toLowerCase();
        if (name.startsWith("on")) el.removeAttribute(attr.name);
        if ((name === "href" || name === "xlink:href") && /^\s*javascript:/i.test(attr.value)) {
          el.removeAttribute(attr.name);
        }
      }
    });

    return new XMLSerializer().serializeToString(svg);
  } catch {
    return raw;
  }
}

// ── mermaid 单例 + 主题跟踪 ────────────────────────────────────────
interface MermaidLike {
  initialize(config: Record<string, unknown>): void;
  render(id: string, code: string): Promise<{ svg: string }>;
}

let mermaidPromise: Promise<MermaidLike> | null = null;
let currentTheme: ThemeName | null = null;
let mermaidId = 0;

async function getMermaid(): Promise<MermaidLike> {
  const theme = detectTheme();
  if (mermaidPromise && currentTheme === theme) return mermaidPromise;

  mermaidPromise = import("mermaid").then((mod) => {
    const m = (mod.default ?? mod) as MermaidLike;
    m.initialize({
      startOnLoad: false,
      theme: theme === "dark" ? "dark" : "default",
      // strict: disables click handlers, HTML labels, and inline scripts in
      // generated SVG. Diagrams are display-only — never an XSS vector for
      // user-pasted markdown.
      securityLevel: "strict",
      fontFamily:
        getComputedStyle(document.documentElement).getPropertyValue("--mono") ||
        "SFMono-Regular, Consolas, monospace",
      themeVariables: { ...THEME_COLORS[theme] },
    });
    currentTheme = theme;
    return m;
  });
  return mermaidPromise;
}

// ── 主题变化监听 ────────────────────────────────────────────────────
// Multiple MermaidViewer instances coexist on a page; each must re-render on
// theme change. Earlier code stored a single onChange callback (the most
// recently mounted viewer overwrote the previous one), so only one diagram
// would update. Use a Set so every active viewer's callback fires.
const themeSubscribers = new Set<() => void>();
let themeObserver: MutationObserver | null = null;
let mediaQuery: MediaQueryList | null = null;
let mediaListener: ((e: MediaQueryListEvent) => void) | null = null;

function notifyThemeChange() {
  mermaidPromise = null;
  currentTheme = null;
  themeSubscribers.forEach((cb) => cb());
}

function startThemeWatch(cb: () => void) {
  themeSubscribers.add(cb);
  if (themeObserver) return;
  themeObserver = new MutationObserver(notifyThemeChange);
  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["data-theme"],
  });
  mediaQuery = window.matchMedia("(prefers-color-scheme: light)");
  mediaListener = () => {
    if (!document.documentElement.hasAttribute("data-theme")) {
      notifyThemeChange();
    }
  };
  mediaQuery.addEventListener("change", mediaListener);
}

function stopThemeWatch(cb: () => void) {
  themeSubscribers.delete(cb);
  if (themeSubscribers.size > 0) return;
  themeObserver?.disconnect();
  themeObserver = null;
  if (mediaQuery && mediaListener) mediaQuery.removeEventListener("change", mediaListener);
  mediaQuery = null;
  mediaListener = null;
}

// ── Mermaid 代码预处理 ──────────────────────────────────────────────
// Mermaid parser 只支持 ASCII 标点作为节点文本的引号/括号，
// 中文全角标点如「」会导致 parse error。渲染前统一替换。
// 注意：不能替换为双引号 "，因为 Mermaid 节点文本 [...] 中的双引号
// 会被解析器当作字符串分隔符，导致 parse error。
function sanitizeMermaidCode(raw: string): string {
  return raw
    // Quote/bracket chars → ASCII single quote (cannot use double — node
    // text [...] treats " as a string delimiter, re-triggering parse errors).
    // Covers: 「 」 『 』, curly quotes, guillemets, chevrons.
    .replace(/[「」『』“”‘’«»《》]/g, "'")
    .replace(/【/g, '[')    // 【
    .replace(/】/g, ']')    // 】
    .replace(/（/g, '(')    // (
    .replace(/）/g, ')')    // )
    .replace(/＜/g, '&lt;') // ＜
    .replace(/＞/g, '&gt;') // ＞
    .replace(/、/g, ',')    // 、
    .replace(/，/g, ',')    // ，
    .replace(/。/g, '.')    // 。
    .replace(/；/g, ';')    // ；
    .replace(/：/g, ':')    // ：
    .replace(/？/g, '?')    // ？
    .replace(/！/g, '!')    // ！
    .replace(/—/g, '-')    // —
    .replace(/…/g, '...')  // …
    .replace(/　/g, ' ');   // ideographic space
}

// ── React 组件 ──────────────────────────────────────────────────────
export function MermaidViewer({ code }: { code: string }) {
  const [svg, setSvg] = useState("");
  const [error, setError] = useState<string | null>(null);

  const renderDiagram = useCallback(async () => {
    try {
      const mermaid = await getMermaid();
      const id = `mermaid-${++mermaidId}`;
      const { svg: raw } = await mermaid.render(id, sanitizeMermaidCode(code.trim()));
      setSvg(sanitizeSvgString(raw));
      setError(null);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
      setSvg("");
    }
  }, [code]);

  useEffect(() => { renderDiagram(); }, [renderDiagram]);

  useEffect(() => {
    const cb = () => {
      setSvg("");
      requestAnimationFrame(() => renderDiagram());
    };
    startThemeWatch(cb);
    return () => stopThemeWatch(cb);
  }, [renderDiagram]);

  if (error) {
    return (
      <div className="mermaid-error">
        <div className="mermaid-error__label">⚠ Mermaid render error</div>
        <pre className="mermaid-error__detail">{error}</pre>
        <pre className="mermaid-error__source"><code>{code}</code></pre>
      </div>
    );
  }
  if (!svg) {
    return <div className="mermaid-loading"><pre><code>{code}</code></pre></div>;
  }
  return <div className="mermaid-diagram" dangerouslySetInnerHTML={{ __html: svg }} />;
}
