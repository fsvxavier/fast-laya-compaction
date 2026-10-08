// Generates golden files for FitState by running the ORIGINAL fast-jev-compaction.
//
//   ORIGINAL_DIR=/path/to/fast-jev-compaction (commit e3f262a, `npm install` done)
//   cd $ORIGINAL_DIR && npx tsx <this repo>/tools/golden/gen.mts <this repo>/internal/compaction/testdata/stages
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

const original = process.env.ORIGINAL_DIR ?? process.cwd();
const { collectToolCalls, fitState } = await import(pathToFileURL(join(original, 'src/index.ts')).href);
const out = process.argv[2];
mkdirSync(out, { recursive: true });

type Msg = { role: 'user' | 'assistant'; text: string; toolUses: any[]; toolResults?: any[] };
const m = (role: Msg['role'], text: string, extra: Partial<Msg> = {}): Msg => ({ role, text, toolUses: [], ...extra });
const call = (id: string, tool: string, input: Record<string, unknown>, text = ''): Msg =>
  m('assistant', '', { toolUses: [{ tool_use_id: id, tool, input, ...(text ? { text } : {}) }] });
const result = (id: string, text: string, isError = false): Msg =>
  m('user', '', { toolResults: [{ tool_use_id: id, text, ...(isError ? { isError } : {}) }] });

const prose = (n: number) => `${n} ` + 'lorem ipsum dolor sit amet, consectetur adipiscing elit. '.repeat(14);

function mixed(): Msg[] {
  const msgs: Msg[] = [m('user', 'Never edit anything under src/generated. Fix the failing test in pkg/<core> & friends.')];
  for (let i = 0; i < 18; i += 1) {
    msgs.push(m('assistant', prose(i)));
    msgs.push(call(`r${i}`, 'Read', { file_path: `/repo/src/module-${i}.ts`, offset: i, limit: 200 }, 'x'.repeat(40)));
    msgs.push(result(`r${i}`, 'content '.repeat(30 + i)));
    if (i % 3 === 0) {
      msgs.push(call(`e${i}`, 'Edit', { file_path: `/repo/src/module-${i}.ts`, old_string: 'a < b && c > d', new_string: 'x'.repeat(900 + i * 10), replace_all: false }, 'ok'));
      msgs.push(result(`e${i}`, 'updated'));
    }
    if (i % 4 === 1) {
      msgs.push(call(`b${i}`, 'Bash', { command: `npm test -- --grep "case ${i}"\n  && echo done`, description: 'run the\ttests' }, 'FAIL'));
      msgs.push(result(`b${i}`, 'FAIL ' + 'stack '.repeat(40), true));
    }
    if (i % 5 === 2) msgs.push(m('user', `follow-up question ${i}: what about the café — “quotes” and ünïcode?`));
  }
  msgs.push(m('assistant', 'The failure is in b.test.ts; fixing now.'));
  msgs.push(m('user', 'go ahead'));
  return msgs;
}

function reads(): Msg[] {
  const msgs: Msg[] = [m('user', 'start')];
  for (let i = 0; i < 60; i += 1) {
    msgs.push(call(`c${i}`, 'Read', { file_path: `/repo/src/module-${i}.ts`, nested: { b: 1, a: [1, 2, { z: 'x y' }] }, flag: i % 2 === 0 }, 'x'));
    msgs.push(result(`c${i}`, 'x'));
  }
  msgs.push(m('assistant', 'done'));
  return msgs;
}

function longTexts(): Msg[] {
  return [
    m('user', prose(0)), m('assistant', prose(1)), m('user', prose(2)), m('assistant', prose(3)),
    call('w', 'Write', { file_path: 'x.ts', content: 'x'.repeat(5000) }, 'ok'), result('w', 'ok'),
    m('assistant', prose(4)), m('user', 'latest'),
  ];
}

const transcripts: Record<string, Msg[]> = { mixed: mixed(), reads: reads(), longTexts: longTexts() };
const fractions = [1, 0.97, 0.9, 0.8, 0.7, 0.6, 0.5, 0.42, 0.35, 0.3, 0.25, 0.2, 0.15, 0.1, 0.06, 0.03];
const preserves = [0, 1, 4];
const seen = new Map<string, number>();

for (const [name, messages] of Object.entries(transcripts)) {
  const cases: unknown[] = [];
  for (const preserveRecentMessages of preserves) {
    const calls = collectToolCalls(messages, preserveRecentMessages);
    const base = fitState(messages, calls, { maxStateTokens: 1e9, preserveRecentMessages, goal: '' });
    for (const f of fractions) {
      const maxStateTokens = Math.max(1, Math.floor(base.tokens * f));
      const options = { maxStateTokens, preserveRecentMessages, goal: '' };
      let expected: unknown;
      try {
        const r = fitState(messages, calls, options);
        expected = { stage: r.stage, tokens: r.tokens, state: r.state };
        seen.set(r.stage, (seen.get(r.stage) ?? 0) + 1);
      } catch (e) {
        expected = { error: true };
        seen.set('<error>', (seen.get('<error>') ?? 0) + 1);
      }
      cases.push({ options, expected });
    }
  }
  writeFileSync(join(out, `${name}.json`), JSON.stringify({ messages, cases }, null, 1) + '\n');
}
console.log(Object.fromEntries(seen));
