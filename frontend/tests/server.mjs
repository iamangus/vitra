import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import http from 'node:http';

mkdirSync('/tmp/opencode', { recursive: true });
const directory = mkdtempSync('/tmp/opencode/vitra-e2e-');
const vault = join(directory, 'vault');
mkdirSync(vault);
writeFileSync(join(vault, 'A.md'), '---\ntype: Note\ntitle: A\n---\n\nOriginal A\n');
writeFileSync(join(vault, 'B.md'), '---\ntype: Note\ntitle: B\n---\n\nOriginal B\n');
const provider = http.createServer(async (request, response) => {
  let body = '';
  for await (const chunk of request) body += chunk;
  const { input } = JSON.parse(body);
  response.setHeader('Content-Type', 'application/json');
  response.end(JSON.stringify({ data: input.map((text, index) => ({ index, embedding: [1, (text.length % 17 + 1) / 20, 0.5] })) }));
});
await new Promise(done => provider.listen(0, '127.0.0.1', done));
const binary = join(directory, 'vitra');
const build = spawnSync('go', ['build', '-o', binary, './cmd/vitra'], { cwd: resolve('..'), env: { ...process.env, GOTOOLCHAIN: 'auto' }, stdio: 'inherit' });
if (build.status !== 0) { provider.close(); rmSync(directory, { recursive: true, force: true }); process.exit(1); }
const app = spawn(binary, [], {
  cwd: resolve('..'),
  env: { ...process.env, VAULT_PATH: vault, CHROMEM_PATH: join(directory, 'index'), BIND_ADDRESS: '127.0.0.1', PORT: '18089', ALLOWED_HOSTS: '', EMBEDDING_API_URL: `http://127.0.0.1:${provider.address().port}`, EMBEDDING_API_KEY: '', EMBEDDING_MODEL: 'fixture' },
  stdio: 'inherit',
});
let stopping = false;
function stop() { if (stopping) return; stopping = true; app.kill('SIGTERM'); }
process.on('SIGINT', stop);
process.on('SIGTERM', stop);
app.on('exit', code => {
  provider.close();
  rmSync(directory, { recursive: true, force: true });
  process.exit(stopping ? 0 : code || 1);
});
