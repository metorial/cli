import { Volume, createFsFromVolume } from 'memfs';

type CommandResult = {
  exitCode: number;
  stdout: string;
  stderr: string;
};

type BrowserRuntime = {
  run(args: string[], env?: Record<string, string>): Promise<CommandResult>;
};

let STORAGE_KEY = 'metorial-cli-browser-fs';
let HOME_DIR = '/home/web';
let WORKSPACE_DIR = '/workspace';

let screen = document.querySelector<HTMLElement>('#screen');
let form = document.querySelector<HTMLFormElement>('#prompt-form');
let input = document.querySelector<HTMLInputElement>('#prompt-input');
let versionNode = document.querySelector<HTMLElement>('#version');

if (!screen || !form || !input || !versionNode) {
  throw new Error('Browser shell UI failed to initialize');
}

let version = versionNode.textContent || 'dev';
let volume = createVolume();
let fs = createFsFromVolume(volume) as any;
let cwd = WORKSPACE_DIR;

bootstrapNodeCompat(fs);
await loadGoRuntime();

let runtime = (globalThis as typeof globalThis & { metorialBrowser?: BrowserRuntime }).metorialBrowser;
if (!runtime) {
  throw new Error('Metorial browser runtime was not registered');
}

printEntry([
  `$ metorial version`,
  `metorial ${version}`,
  '',
  'Try commands like `version`, `providers list`, or `login`.',
  'Type `clear` to reset the screen.'
], ['command', 'muted', 'muted', 'muted', 'muted']);

form.addEventListener('submit', async event => {
  event.preventDefault();

  let commandText = input.value.trim();
  if (!commandText) {
    return;
  }

  input.value = '';

  if (commandText === 'clear') {
    screen.innerHTML = '';
    return;
  }

  let args = splitCommand(commandText);
  printEntry([`$ metorial ${commandText}`], ['command']);
  setPending(true);

  try {
    let result = await runtime.run(args, {
      HOME: HOME_DIR,
      METORIAL_SKIP_UPDATE_CHECK: '1'
    });

    persistVolume();
    renderCommandResult(result);
  } catch (error) {
    let message = error instanceof Error ? error.message : String(error);
    printEntry([message], ['stderr']);
  } finally {
    setPending(false);
    input.focus();
  }
});

input.focus();

function createVolume() {
  let stored = localStorage.getItem(STORAGE_KEY);
  let snapshot = stored ? JSON.parse(stored) : {};
  let volume = Volume.fromJSON(snapshot, '/');

  tryMkdir(volume, HOME_DIR);
  tryMkdir(volume, `${HOME_DIR}/.metorial`);
  tryMkdir(volume, `${HOME_DIR}/.metorial/cli`);
  tryMkdir(volume, WORKSPACE_DIR);

  return volume;
}

function bootstrapNodeCompat(fs: any) {
  (globalThis as any).fs = fs;
  (globalThis as any).process = {
    env: {
      HOME: HOME_DIR
    },
    getuid() {
      return -1;
    },
    getgid() {
      return -1;
    },
    geteuid() {
      return -1;
    },
    getegid() {
      return -1;
    },
    getgroups() {
      throw new Error('not implemented');
    },
    pid: 1,
    ppid: 1,
    umask() {
      return 0;
    },
    cwd() {
      return cwd;
    },
    chdir(nextDir: string) {
      cwd = normalizePath(nextDir);
    }
  };
}

async function loadGoRuntime() {
  await loadScript('./browser-shell-wasm_exec.js');

  let GoConstructor = (globalThis as any).Go;
  if (!GoConstructor) {
    throw new Error('Go WASM runtime failed to load');
  }

  let go = new GoConstructor();
  let response = await fetch('./browser-shell-metorial.wasm');
  let bytes = await response.arrayBuffer();
  let result = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(result.instance);
}

function loadScript(source: string) {
  return new Promise<void>((resolve, reject) => {
    let script = document.createElement('script');
    script.src = source;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error(`Failed to load ${source}`));
    document.head.appendChild(script);
  });
}

function renderCommandResult(result: CommandResult) {
  let lines: string[] = [];
  let classes: string[] = [];

  if (result.stdout.trim()) {
    for (let line of result.stdout.replaceAll('\r\n', '\n').split('\n')) {
      if (!line) {
        continue;
      }
      lines.push(line);
      classes.push('stdout');
    }
  }

  if (result.stderr.trim()) {
    for (let line of result.stderr.replaceAll('\r\n', '\n').split('\n')) {
      if (!line) {
        continue;
      }
      lines.push(line);
      classes.push('stderr');
    }
  }

  if (lines.length === 0) {
    lines.push(result.exitCode === 0 ? 'Command completed.' : `Command failed with exit code ${result.exitCode}.`);
    classes.push('muted');
  }

  printEntry(lines, classes);
}

function printEntry(lines: string[], classes: string[]) {
  let entry = document.createElement('div');
  entry.className = 'shell__entry';

  lines.forEach((line, index) => {
    let row = document.createElement('div');
    row.className = `shell__line shell__line--${classes[index] || 'stdout'}`;
    row.textContent = line;
    entry.appendChild(row);
  });

  screen?.appendChild(entry);
  screen?.scrollTo({ top: screen.scrollHeight });
}

function setPending(value: boolean) {
  input.disabled = value;
  let button = form.querySelector<HTMLButtonElement>('button');
  if (button) {
    button.disabled = value;
    button.textContent = value ? 'Running...' : 'Run';
  }
}

function persistVolume() {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(volume.toJSON()));
}

function normalizePath(value: string) {
  if (value.startsWith('/')) {
    return value;
  }

  return `${cwd.replace(/\/$/, '')}/${value}`.replace(/\/+/g, '/');
}

function tryMkdir(volume: Volume, dir: string) {
  try {
    volume.mkdirSync(dir, { recursive: true });
  } catch {}
}

function splitCommand(input: string) {
  let tokens: string[] = [];
  let current = '';
  let quote = '';

  for (let index = 0; index < input.length; index++) {
    let char = input[index];

    if (quote) {
      if (char === quote) {
        quote = '';
        continue;
      }

      if (char === '\\' && index + 1 < input.length) {
        current += input[index + 1];
        index += 1;
        continue;
      }

      current += char;
      continue;
    }

    if (char === '"' || char === "'") {
      quote = char;
      continue;
    }

    if (/\s/.test(char)) {
      if (current) {
        tokens.push(current);
        current = '';
      }
      continue;
    }

    current += char;
  }

  if (current) {
    tokens.push(current);
  }

  return tokens;
}
