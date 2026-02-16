import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

export type AppMode = 'full' | 'standalone' | 'remote';

export interface RemoteConnection {
  id: string;
  name: string;
  url: string;
  token: string;
}

export interface ElectronConfig {
  mode: AppMode | '';
  sidecarPath: string;
  remoteConnections: RemoteConnection[];
  windowState: { width: number; height: number; x?: number; y?: number };
}

const CONFIG_DIR = path.join(os.homedir(), '.gomikrobot');
const CONFIG_FILE = path.join(CONFIG_DIR, 'electron.json');

function defaultConfig(): ElectronConfig {
  return {
    mode: '',
    sidecarPath: '',
    remoteConnections: [],
    windowState: { width: 1400, height: 900 },
  };
}

export function loadElectronConfig(): ElectronConfig {
  try {
    const data = fs.readFileSync(CONFIG_FILE, 'utf-8');
    return { ...defaultConfig(), ...JSON.parse(data) };
  } catch {
    return defaultConfig();
  }
}

export function saveElectronConfig(cfg: ElectronConfig): void {
  fs.mkdirSync(CONFIG_DIR, { recursive: true });
  fs.writeFileSync(CONFIG_FILE, JSON.stringify(cfg, null, 2), 'utf-8');
}

/**
 * Resolve the app mode from CLI args, config file, or return '' to show picker.
 */
export function resolveMode(argv: string[]): AppMode | '' {
  // Check for --reset-mode: clear persisted mode and show picker
  if (argv.includes('--reset-mode')) {
    const cfg = loadElectronConfig();
    cfg.mode = '';
    saveElectronConfig(cfg);
    return '';
  }

  // Check CLI args: --mode=full|standalone|remote
  for (const arg of argv) {
    const match = arg.match(/^--mode=(full|standalone|remote)$/);
    if (match) {
      return match[1] as AppMode;
    }
  }

  // Check persisted config
  const cfg = loadElectronConfig();
  if (cfg.mode === 'full' || cfg.mode === 'standalone' || cfg.mode === 'remote') {
    return cfg.mode;
  }

  // No mode resolved — show picker
  return '';
}
