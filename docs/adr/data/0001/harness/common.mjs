import { execFileSync, spawn } from 'node:child_process';
import { openSync, writeSync, closeSync } from 'node:fs';
import { access, readFile, realpath, rm, mkdir, readdir, copyFile } from 'node:fs/promises';
import { join, dirname, resolve } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';

export const repoRoot = fileURLToPath(new URL('../../../../../', import.meta.url));
export const MINUTE_MS = 60_000;

export function positiveInteger(value, name) {
  const number = Number(value);
  if (!Number.isSafeInteger(number) || number <= 0) throw new Error(`${name} must be a positive integer`);
  return number;
}

export function launch(command, args, options = {}) {
  const { log, ...spawnOptions } = options;
  // Open before spawning; sync writes keep small measurement logs durable and errors observable.
  const descriptor = log ? openSync(log, 'a') : null;
  const child = spawn(command, args, { stdio: ['pipe', 'pipe', 'pipe'], ...spawnOptions });
  let stdout = '';
  let logError;
  const writeLog = (chunk) => {
    if (descriptor === null || logError) return;
    try { writeSync(descriptor, chunk); }
    catch (error) { logError = error; child.kill('SIGTERM'); }
  };
  child.stdout.on('data', (chunk) => { stdout += chunk; writeLog(chunk); });
  child.stderr.on('data', writeLog);
  const done = new Promise((resolveDone, reject) => {
    child.once('error', reject);
    child.once('close', (code, signal) => {
      if (descriptor !== null) {
        try { closeSync(descriptor); } catch (error) { logError ??= error; }
      }
      if (logError) reject(new Error(`cannot write ${log}: ${logError.message}`));
      else if (code === 0) resolveDone(stdout);
      else reject(new Error(`${command} exited with ${signal ?? code}${log ? `; see ${log}` : ''}`));
    });
  });
  // The final owner awaits done; observe rejection while it waits for protocol events.
  done.catch(() => {});
  return { child, done };
}

export async function execute(command, args, options = {}) {
  const { timeoutMs, cancelled, ...launchOptions } = options;
  const subprocess = launch(command, args, launchOptions);
  subprocess.child.stdin.end();
  const completion = cancelled ? Promise.race([subprocess.done, cancelled]) : subprocess.done;
  try {
    return await (timeoutMs ? withTimeout(completion, timeoutMs, command) : completion);
  } finally {
    if (subprocess.child.exitCode === null && subprocess.child.signalCode === null) subprocess.child.kill('SIGKILL');
    await subprocess.done.catch(() => {});
  }
}

export async function withTimeout(promise, timeoutMs, description) {
  let timer;
  try {
    return await Promise.race([
      promise,
      new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(`${description} timed out after ${timeoutMs} ms`)), timeoutMs); }),
    ]);
  } finally {
    clearTimeout(timer);
  }
}

export async function stopEngine(process) {
  if (process.child.exitCode !== null || process.child.signalCode !== null) return process.done;
  process.child.stdin.end();
  try {
    await withTimeout(process.done, 10_000, 'engine shutdown');
  } catch (error) {
    process.child.kill('SIGKILL');
    throw error;
  }
}

function hasLiveGroupMember(pgid) {
  const rows = execFileSync('ps', ['-A', '-o', 'pgid=,stat='], { encoding: 'utf8' }).trim().split('\n');
  return rows.some((row) => {
    const [group, stat] = row.trim().split(/\s+/);
    return Number(group) === pgid && !stat.startsWith('Z');
  });
}

function signalGroup(pid, signal) {
  try { process.kill(-pid, signal); return true; }
  catch (error) {
    if (error.code === 'ESRCH') return false;
    // Darwin reports EPERM, not ESRCH, for a group whose only members are zombies.
    if (error.code === 'EPERM' && !hasLiveGroupMember(pid)) return false;
    throw error;
  }
}

/** Terminate all children of a detached launch, even when the original root has exited. */
export async function stopProcessGroup(ownedProcess) {
  const pid = ownedProcess.child.pid;
  if (!pid) { await ownedProcess.done.catch(() => {}); return; }
  if (!signalGroup(pid, 'SIGTERM')) return;
  const deadline = Date.now() + 10_000;
  while (signalGroup(pid, 0) && Date.now() < deadline) await sleep(50);
  if (signalGroup(pid, 0)) {
    signalGroup(pid, 'SIGKILL');
    const killDeadline = Date.now() + 5000;
    while (signalGroup(pid, 0) && Date.now() < killDeadline) await sleep(50);
    if (signalGroup(pid, 0)) throw new Error(`process group ${pid} survived SIGKILL`);
  }
  await ownedProcess.done.catch(() => {});
}

/** Reset only profiles in a marked driver-owned output, after validating all seed inputs. */
export async function resetUserData({ seed, userData, ownerRoot }) {
  const root = await realpath(ownerRoot);
  const parent = await realpath(dirname(userData));
  const seedRoot = await realpath(seed);
  const marker = JSON.parse(await readFile(join(root, '.harness-owner.json'), 'utf8'));
  if (marker.root !== root || resolve(userData) !== join(parent, 'user-data') || !parent.startsWith(`${root}/`)) throw new Error('profile is not inside its marked driver output');
  if (seedRoot === root || seedRoot.startsWith(`${root}/`) || root.startsWith(`${seedRoot}/`)) throw new Error('seed must be outside driver output');
  for (const name of ['sync.db', 'settings.json']) await access(join(seedRoot, name));
  await rm(userData, { recursive: true, force: true });
  await mkdir(userData);
  for (const name of ['sync.db', 'settings.json']) await copyFile(join(seedRoot, name), join(userData, name));
  for (const name of await readdir(seedRoot)) if (name === 'sync.db-wal') await copyFile(join(seedRoot, name), join(userData, name));
}

export function quoteShell(value) {
  return `'${String(value).replaceAll("'", "'\\''")}'`;
}
