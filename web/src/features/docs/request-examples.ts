/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export function buildRequestExamples(baseUrl: string) {
  return {
    curl: `curl "${baseUrl}/chat/completions" \\
  -H "Authorization: Bearer $OMNIVO_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "MODEL_ID",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'`,
    python: `import os
from openai import OpenAI

client = OpenAI(
    api_key=os.environ["OMNIVO_API_KEY"],
    base_url="${baseUrl}",
)
response = client.chat.completions.create(
    model="MODEL_ID",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(response.choices[0].message.content)`,
    javascript: `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: process.env.OMNIVO_API_KEY,
  baseURL: "${baseUrl}",
});
const response = await client.chat.completions.create({
  model: "MODEL_ID",
  messages: [{ role: "user", content: "Hello!" }],
});
console.log(response.choices[0].message.content);`,
    stream: `curl --no-buffer "${baseUrl}/chat/completions" \\
  -H "Authorization: Bearer $OMNIVO_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "MODEL_ID",
    "stream": true,
    "messages": [{"role": "user", "content": "Hello!"}]
  }'`,
  }
}
