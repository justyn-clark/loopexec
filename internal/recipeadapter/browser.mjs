import { readFileSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { resolve, join, sep } from "node:path";
import { pathToFileURL } from "node:url";
const c = JSON.parse(readFileSync(process.argv[2], "utf8"));
const require = createRequire(import.meta.url);
const { chromium } = require(c.adapter.tools?.playwright || "playwright");
const options = c.adapter.options || {};
const result = {
  checks: [],
  metrics: {},
  files: [],
  tools: ["playwright/chromium"],
};
const add = (id, passed, detail = "") =>
  result.checks.push({ id, passed, detail });
const browser = await chromium.launch({ headless: true });
try {
  const context = await browser.newContext({
    viewport: options.viewport || { width: 960, height: 640 },
    deviceScaleFactor: 1,
    locale: "en-US",
    reducedMotion: "reduce",
  });
  const page = await context.newPage();
  page.setDefaultTimeout(c.timeout_seconds * 1000);
  const root = resolve(c.root, c.inputs.project || ".");
  const localURL = (route) => {
    let path = resolve(root, route.replace(/^\/+/, "") || "index.html");
    if (path !== root && !path.startsWith(root + sep))
      throw Error("route escapes project");
    if (!path.includes(".html")) path = join(path, "index.html");
    return pathToFileURL(path).href;
  };
  const base = c.inputs.page_url || options.base_url || c.inputs.base_url;
  const routes =
    c.inputs.routes || c.inputs.pages || (c.inputs.page_url ? [""] : ["/"]);
  const viewports = c.inputs.viewports || ["960x640"];
  let index = 0;
  for (const route of routes) {
    const url =
      base && !base.startsWith("file:")
        ? new URL(route, base).href
        : c.inputs.page_url
          ? base
          : localURL(route);
    for (const viewport of viewports) {
      if (c.recipe === "responsive-ui") {
        const [width, height] = viewport.split("x").map(Number);
        if (!width || !height) throw Error("invalid viewport");
        await page.setViewportSize({ width, height });
      }
      await page.goto(url, {
        waitUntil: "networkidle",
        timeout: c.timeout_seconds * 1000,
      });
      await page.evaluate(() => document.fonts.ready);
      const path = join(
        c.evidence_dir,
        "page-" + String(index++).padStart(3, "0") + ".png",
      );
      await page.screenshot({
        path,
        animations: "disabled",
        caret: "hide",
        fullPage: false,
      });
      result.files.push(path);
      add("page-loaded:" + route + ":" + viewport, true);
      for (const rule of options.assertions || []) {
        const locator = page.locator(rule.selector);
        let actual;
        if (rule.property === "count") actual = await locator.count();
        else if (rule.property === "text") actual = await locator.textContent();
        else if (rule.property === "visible")
          actual = await locator.isVisible();
        else
          actual = await locator.evaluate((element, property) => {
            const rect = element.getBoundingClientRect();
            if (property.startsWith("rect.")) return rect[property.slice(5)];
            if (property.startsWith("style."))
              return getComputedStyle(element)[property.slice(6)];
            return element.getAttribute(property);
          }, rule.property);
        const passed =
          ("equals" in rule ? actual === rule.equals : true) &&
          ("min" in rule ? actual >= rule.min : true) &&
          ("max" in rule ? actual <= rule.max : true);
        add(
          "dom:" + route + ":" + viewport + ":" + rule.id,
          passed,
          JSON.stringify({ actual }),
        );
      }
      if (c.recipe === "responsive-ui") {
        const overflow = await page.evaluate(() =>
          Math.max(0, document.documentElement.scrollWidth - innerWidth),
        );
        result.metrics["overflow:" + route + ":" + viewport] = overflow;
        add(
          "overflow:" + route + ":" + viewport,
          overflow <= c.inputs.max_overflow_px,
        );
      }
      if (c.recipe === "accessibility") {
        const axe = c.adapter.tools?.axe;
        if (!axe)
          throw Error(
            "accessibility requires tools.axe pointing to axe.min.js",
          );
        await page.addScriptTag({ path: axe });
        const audit = await page.evaluate(
          async (rules) =>
            await axe.run(document, {
              runOnly: { type: "rule", values: rules },
            }),
          c.inputs.rules,
        );
        writeFileSync(
          join(c.evidence_dir, "axe-" + index + ".json"),
          JSON.stringify(audit, null, 2),
        );
        const critical = audit.violations.filter(
          (v) => v.impact === "critical",
        ).length;
        result.metrics["critical:" + route] = critical;
        add("axe-critical:" + route, critical <= c.inputs.max_critical);
        for (const violation of audit.violations)
          if (violation.impact === "critical")
            add("axe:" + route + ":" + violation.id, false);
      }
      if (c.recipe === "seo-metadata" || c.recipe === "static-site") {
        const head = await page.evaluate(() => ({
          title: document.title,
          canonical: document.querySelector('link[rel="canonical"]')?.href,
          ogTitle: document.querySelector('meta[property="og:title"]')?.content,
          ogImage: document.querySelector('meta[property="og:image"]')?.content,
          description: document.querySelector('meta[name="description"]')
            ?.content,
          links: [
            ...document.querySelectorAll(
              "a[href],img[src],script[src],link[href]",
            ),
          ].map((e) => e.href || e.src),
        }));
        const required =
          c.inputs.metadata_rules || c.inputs.required_metadata || [];
        for (const rule of required) {
          if (rule === "title")
            add(
              "title:" + route,
              Boolean(head.title) &&
                head.title.length <= (options.max_title_length || 60),
            );
          else if (rule === "canonical")
            add(
              "canonical:" + route,
              Boolean(head.canonical) &&
                head.canonical.startsWith(
                  c.inputs.origin || options.canonical_origin || "https://",
                ),
            );
          else if (rule === "og")
            add("og:" + route, Boolean(head.ogTitle && head.ogImage));
          else if (rule === "description")
            add("description:" + route, Boolean(head.description));
          else if (!["links", "sitemap"].includes(rule))
            throw Error("unknown metadata rule " + rule);
        }
        for (const link of head.links) {
          if (link.startsWith("file:")) {
            try {
              readFileSync(new URL(link));
              add("asset:" + route + ":" + link, true);
            } catch {
              add("asset:" + route + ":" + link, false);
            }
          } else if (new URL(link).origin === new URL(url).origin) {
            const response = await context.request.get(link);
            add("link:" + route + ":" + link, response.ok());
          }
        }
        if (required.includes("sitemap")) {
          const sitemap = options.sitemap;
          if (!sitemap) throw Error("sitemap option required");
          const data = readFileSync(resolve(c.root, sitemap), "utf8");
          add(
            "sitemap:" + route,
            data.includes(new URL(route, c.inputs.origin).href),
          );
        }
      }
    }
  }
  await context.close();
} finally {
  await browser.close();
}
process.stdout.write(JSON.stringify(result));
