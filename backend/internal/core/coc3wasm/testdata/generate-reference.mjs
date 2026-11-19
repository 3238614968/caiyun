// Development-only oracle. Production and tests do not execute this script.
import fs from 'node:fs';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';

const assets = new URL('../../tasks/assets/coc3/', import.meta.url);
const javascript = fs.readFileSync(new URL('wheel.js', assets));
const wasm = fs.readFileSync(new URL('wheel_bg.wasm', assets));
const stack = 'Error\n    at wheel.roto_simple (coc3-wheel.wasm)';

// Match the Go host: no browser or process global, no environment variables,
// no networking, and only the expression used by the official wheel ABI.
globalThis.Window = class Window {};
globalThis.window = new Window();
globalThis.window.navigator = Object.freeze({
  userAgent: 'Mozilla/5.0 (Linux; Android 16; RMQ) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/120.0.0.0 Mobile Safari/537.36 leadeon/12.5.4/CMCCIT',
  platform: 'Linux aarch64'
});
globalThis.window.location = Object.freeze({ href: 'https://dev.coc.10086.cn/coc3/canvas/rightsmarket-h5-canvas/online/home' });
globalThis.self = globalThis.window;
globalThis.eval = source => {
  if (source !== 'new Error().stack') throw new Error('Unsupported expression');
  return stack;
};
globalThis.fetch = async () => { throw new Error('Network access is disabled'); };

const vectors = [];
for (const [index, config] of [
  { clock: 1794398400123, seed: 0 },
  { clock: 1794398401223, seed: 17 }
].entries()) {
  let next = config.seed;
  let randomBytes = 0;
  Date.now = () => config.clock;
  Object.defineProperty(globalThis, 'crypto', {
    configurable: true,
    value: { getRandomValues(buffer) {
      for (let i = 0; i < buffer.length; i++) buffer[i] = next++ & 255;
      randomBytes += buffer.length;
      return buffer;
    } }
  });
  const module = await import(`data:text/javascript;base64,${javascript.toString('base64')}#vector-${index}`);
  await module.default({ module_or_path: wasm });
  const wheel = module.Wheel.create_wheel();
  try { vectors.push({ ...config, randomBytes, pair: wheel.roto_simple('{}') }); }
  finally { wheel.free(); }
  vectors.at(-1).randomBytes = randomBytes;
}

const reference = {
  wasmSHA256: createHash('sha256').update(wasm).digest('hex'),
  javascriptSHA256: createHash('sha256').update(javascript).digest('hex'),
  stack,
  vectors
};
const target = new URL('reference.json', import.meta.url);
fs.writeFileSync(target, JSON.stringify(reference, null, 2) + '\n');
console.log(`Wrote ${vectors.length} isolated reference vectors to ${fileURLToPath(target)}`);
