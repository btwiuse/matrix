#!/usr/bin/env node
// HTML 注入反向代理:转发请求到上游,响应时把注入片段插到 </body> 前。
// 用法:
//   node server.js --upstream http://127.0.0.1:8000 --port 8080 --inject ./inject.html
//   node server.js --upstream http://127.0.0.1:8000 --port 8080 --inject-html '<script src="/x.js"></script>'

import http from "node:http";
import https from "node:https";
import fs from "node:fs";
import crypto from "node:crypto";
import { URL } from "node:url";

// --- 参数解析 ---
const args = {};
const argv = process.argv.slice(2);
for (let i = 0; i < argv.length; i++) {
  const k = argv[i];
  if (!k.startsWith("--")) continue;
  const name = k.slice(2);
  const next = argv[i + 1];
  if (next && !next.startsWith("--")) {
    args[name] = next;
    i++;
  } else args[name] = true;
}

const upstream = new URL(args.upstream || "http://127.0.0.1:8000");
const port = parseInt(args.port || "8080", 10);
const verbose = args.verbose !== undefined;

let injection = args["inject-html"] || "";
if (args.inject) injection = fs.readFileSync(args.inject, "utf8");
if (!injection.trim()) {
  console.error("缺少注入内容:用 --inject <file> 或 --inject-html <html> 提供");
  process.exit(1);
}

// 幂等标记:按注入内容哈希生成,检测到已注入则跳过
const marker = `\n<!-- html-inject:${
  crypto.createHash("sha256").update(injection).digest("hex").slice(0, 12)
} -->`;

function injectInto(html) {
  if (html.includes(marker.trim())) return html; // 已注入,幂等
  const body = html.lastIndexOf("</body>"); // 首选 </body> 前
  if (body !== -1) {
    return html.slice(0, body) + marker + "\n" + injection + "\n" +
      html.slice(body);
  }
  const doc = html.lastIndexOf("</html>"); // 兜底 </html> 前
  if (doc !== -1) {
    return html.slice(0, doc) + marker + "\n" + injection + "\n" +
      html.slice(doc);
  }
  return html + marker + "\n" + injection; // 最后兜底:追加末尾
}

// --- 代理服务器 ---
const server = http.createServer((req, res) => {
  const url = new URL(req.url, upstream.href);
  const headers = { ...req.headers };
  delete headers.host;
  delete headers["accept-encoding"]; // 关掉压缩,便于解析/注入
  headers.host = upstream.host;

  const lib = upstream.protocol === "https:" ? https : http;
  const preq = lib.request(url, { method: req.method, headers }, (pres) => {
    const ct = pres.headers["content-type"] || "";
    const chunks = [];
    pres.on("data", (c) => chunks.push(c));
    pres.on("end", () => {
      const raw = Buffer.concat(chunks);
      let outBody = raw;
      const outHeaders = { ...pres.headers };

      if (ct.includes("text/html")) {
        const text = raw.toString("utf8");
        const injected = injectInto(text);
        if (injected !== text) {
          outBody = Buffer.from(injected, "utf8");
          delete outHeaders["content-encoding"];
          delete outHeaders["content-length"];
          delete outHeaders["transfer-encoding"];
          delete outHeaders["etag"];
          outHeaders["content-length"] = outBody.length;
          if (verbose) {
            console.log(
              `[inject] ${req.method} ${req.url} (${raw.length} -> ${outBody.length} 字节)`,
            );
          }
        } else if (verbose) {
          console.log(`[skip]   ${req.method} ${req.url} 已注入,跳过`);
        }
      } else if (verbose) {
        console.log(
          `[pass]   ${req.method} ${req.url} 非 text/html (${
            ct || "无 Content-Type"
          })`,
        );
      }

      res.writeHead(pres.statusCode || 200, outHeaders);
      res.end(outBody);
    });
  });
  preq.on("error", (e) => {
    res.writeHead(502);
    res.end("Bad gateway: " + e.message);
  });
  req.pipe(preq);
});

server.listen(port, () => {
  console.log(`注入代理监听 :${port} -> ${upstream.href}`);
  console.log(`注入片段 ${injection.length} 字节,标记 ${marker.trim()}`);
});
