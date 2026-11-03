// Local OpenAI-compatible upstream for end-to-end compression tests.
import { createServer } from "node:http";
import { writeFileSync } from "node:fs";

const port = Number(process.env.MOCK_UPSTREAM_PORT || 19000);
const responseModel = process.env.MOCK_RESPONSE_MODEL || "gpt-4o";

createServer((req, res) => {
  let body = "";
  req.on("data", (chunk) => (body += chunk));
  req.on("end", () => {
    try {
      writeFileSync("testdata/upstream-last.json", body);
    } catch {
      // best effort
    }
    if (req.url?.includes("/responses")) {
      const payload = {
        id: "resp_mock",
        object: "response",
        model: responseModel,
        status: "completed",
        output: [{ type: "message", role: "assistant", content: [{ type: "output_text", text: "ok" }] }],
        usage: { input_tokens: 100, output_tokens: 5, total_tokens: 105 },
      };
      res.writeHead(200, { "content-type": "application/json" });
      res.end(JSON.stringify(payload));
      return;
    }
    const payload = {
      id: "chatcmpl-mock",
      object: "chat.completion",
      created: Math.floor(Date.now() / 1000),
      model: responseModel,
      choices: [{ index: 0, message: { role: "assistant", content: "ok" }, finish_reason: "stop" }],
      usage: { prompt_tokens: 100, completion_tokens: 5, total_tokens: 105 },
    };
    res.writeHead(200, { "content-type": "application/json" });
    res.end(JSON.stringify(payload));
  });
}).listen(port, "127.0.0.1", () => console.log(`mock upstream on http://127.0.0.1:${port}`));
