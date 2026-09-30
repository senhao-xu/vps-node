import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { readFileSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { URL } from 'node:url'
import ts from 'typescript'
import { computed, effectScope, reactive, ref, watch } from 'vue'

function component(t, name, input, mocks, expose) {
  const sourceFile = readFileSync(new URL(`../src/components/${name}.vue`, import.meta.url), 'utf8')
  let source = sourceFile.match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1]
  const ast = ts.createSourceFile(`${name}.ts`, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
  for (const node of [...ast.statements].reverse()) {
    if (ts.isImportDeclaration(node)) source = source.slice(0, node.getFullStart()) + source.slice(node.end)
  }
  const props = reactive(input)
  const emitted = []
  const env = {
    computed, ref, watch,
    defineProps: () => props,
    defineEmits: () => (...args) => emitted.push(args),
    withDefaults: (value, defaults) => {
      for (const [key, fallback] of Object.entries(defaults)) {
        if (value[key] === undefined) value[key] = fallback
      }
      return value
    },
    errorMessage: err => String(err),
    ...mocks,
  }
  const js = ts.transpileModule(`${source}\nreturn {${expose.join(',')}}`, {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  }).outputText
  const scope = effectScope()
  t.after(() => scope.stop())
  const api = scope.run(() => new Function(...Object.keys(env), js)(...Object.values(env)))
  return { ...api, props, emitted }
}

function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

function shareDialog(t, qr = async link => `qr:${link}`) {
  const requests = []
  const api = component(t, 'NodeShareDialog', { open: true, node: { id: 17 } }, {
    getNodeShare(nodeId, userId) {
      const pending = deferred()
      requests.push({ nodeId, userId, ...pending })
      return pending.promise
    },
    toDataURL: qr,
    listUsers: async () => ({ items: [] }),
  }, ['selectUser', 'share', 'qrUrls', 'shareError', 'loadingShare', 'selectedUserId'])
  return { ...api, requests }
}

const alice = { id: 1, username: 'alice' }
const bob = { id: 2, username: 'bob' }
const result = (userId, nodeId = 17) => ({ node_id: nodeId, user_id: userId, authorized: true, links: [`credential-${userId}`] })

test('late share response cannot overwrite the selected user', async t => {
  const ui = shareDialog(t)
  const first = ui.selectUser(alice)
  const second = ui.selectUser(bob)
  ui.requests[1].resolve(result(2))
  await second
  ui.requests[0].resolve(result(1))
  await first
  assert.equal(ui.selectedUserId.value, 2)
  assert.equal(ui.share.value.user_id, 2)
  assert.deepEqual(ui.qrUrls.value, ['qr:credential-2'])
})

test('late QR generation cannot overwrite the selected user', async t => {
  const qr = deferred()
  const started = deferred()
  const ui = shareDialog(t, link => {
    if (link === 'credential-1') { started.resolve(); return qr.promise }
    return Promise.resolve(`qr:${link}`)
  })
  const first = ui.selectUser(alice)
  ui.requests[0].resolve(result(1))
  await started.promise
  const second = ui.selectUser(bob)
  ui.requests[1].resolve(result(2))
  await second
  qr.resolve('qr:credential-1')
  await first
  assert.equal(ui.share.value.user_id, 2)
  assert.deepEqual(ui.qrUrls.value, ['qr:credential-2'])
})

test('stale error and finally leave the newer request loading', async t => {
  const ui = shareDialog(t)
  const first = ui.selectUser(alice)
  const second = ui.selectUser(bob)
  ui.requests[0].reject(new Error('old request failed'))
  await first
  assert.equal(ui.shareError.value, '')
  assert.equal(ui.loadingShare.value, true)
  ui.requests[1].resolve(result(2))
  await second
  assert.equal(ui.loadingShare.value, false)
})

test('closing and reopening invalidates even a request for the same user', async t => {
  const ui = shareDialog(t)
  const first = ui.selectUser(alice)
  ui.props.open = false
  ui.props.open = true
  const second = ui.selectUser(alice)
  ui.requests[0].resolve(result(1))
  await first
  assert.equal(ui.share.value, null)
  assert.equal(ui.loadingShare.value, true)
  ui.requests[1].resolve(result(1))
  await second
  assert.equal(ui.share.value.user_id, 1)
})

test('changing the node invalidates pending credentials', async t => {
  const ui = shareDialog(t)
  const first = ui.selectUser(alice)
  ui.props.node = { id: 18 }
  ui.requests[0].resolve(result(1))
  await first
  assert.equal(ui.share.value, null)
  assert.equal(ui.selectedUserId.value, null)
  const second = ui.selectUser(alice)
  assert.equal(ui.requests[1].nodeId, 18)
  ui.requests[1].resolve(result(1, 18))
  await second
  assert.equal(ui.share.value.node_id, 18)
})

test('disabled authorization can be removed but cannot be added', t => {
  const node = { id: 7, status: 'disabled' }
  const ui = component(t, 'NodeChecklist', { nodes: [node], servers: [], modelValue: [7, 8] }, {}, ['toggle'])
  ui.toggle(node, false)
  assert.deepEqual(ui.emitted, [['update:modelValue', [8]]])
  ui.props.modelValue = [8]
  ui.toggle(node, true)
  assert.equal(ui.emitted.length, 1)
})

test('clearing Hysteria2 hopping explicitly removes the saved value', t => {
  const ui = component(t, 'NodeFormDialog', { open: true, node: { id: 1 } }, {
    SHADOWSOCKS_METHODS: ['2022-blake3-aes-128-gcm'], protocolLabel: value => value,
  }, ['isEdit', 'detailLoaded', 'protocol', 'applyNodeSettings', 'hy2HopInterval', 'settingsPayload'])
  ui.isEdit.value = true
  ui.detailLoaded.value = true
  ui.protocol.value = 'hysteria2'
  ui.applyNodeSettings({ tls: { server_name: 'example.com' }, hop_interval: '30000-40000' })
  ui.hy2HopInterval.value = ''
  assert.equal(ui.settingsPayload.value.hop_interval, '')
})

function installCommands() {
  const source = readFileSync(new URL('../src/utils/installCommands.ts', import.meta.url), 'utf8')
  const js = ts.transpileModule(source, {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  }).outputText
  const exports = {}
  new Function('exports', js)(exports)
  return exports
}

test('binary installation passes literal credentials to the installer shell', t => {
  const dir = mkdtempSync(join(tmpdir(), 'vps-install-command-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  const script = '#!/bin/sh\nprintf "%s\\n" "$PANEL_URL" "$SERVER_ID" "$AGENT_KEY"\n'
  writeFileSync(join(dir, 'curl'), `#!/bin/sh\n[ "$1" = -fsSL ] || exit 10\n[ "$3" = -o ] || exit 11\nprintf '%s' '${script}' > "$4"\n`, { mode: 0o700 })
  const origin = "https://panel.example/path with 'quote'"
  const key = "abc' $(exit 99) `exit 98` \"xyz\""
  const command = installCommands().binaryInstallCommand(origin, 23, key)
  const output = execFileSync('/bin/sh', ['-c', command], { env: { PATH: `${dir}:/usr/bin:/bin` }, encoding: 'utf8' })
  assert.equal(output, `${origin}\n23\n${key}\n`)
})

test('a failed installer download never executes the shell', t => {
  const dir = mkdtempSync(join(tmpdir(), 'vps-install-fail-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  writeFileSync(join(dir, 'curl'), '#!/bin/sh\nexit 42\n', { mode: 0o700 })
  assert.throws(() => execFileSync('/bin/sh', ['-c', installCommands().binaryInstallCommand('https://panel.example', 1, 'key')], {
    env: { PATH: `${dir}:/usr/bin:/bin` }, encoding: 'utf8',
  }), err => err.status === 42)
})
