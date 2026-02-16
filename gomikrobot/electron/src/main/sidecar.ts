import { ChildProcess, spawn } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import * as http from 'http';
import { AppMode } from './mode-resolver';

const HEALTH_URL = 'http://127.0.0.1:18791/api/v1/status';
const HEALTH_INTERVAL_MS = 2000;
const STARTUP_TIMEOUT_MS = 30000;
const SHUTDOWN_TIMEOUT_MS = 5000;

export type SidecarStatus = 'stopped' | 'starting' | 'running' | 'error';

export class SidecarManager {
  private proc: ChildProcess | null = null;
  private status: SidecarStatus = 'stopped';
  private healthTimer: ReturnType<typeof setInterval> | null = null;
  private onStatusChange: ((status: SidecarStatus) => void) | null = null;
  private logs: string[] = [];

  setStatusCallback(cb: (status: SidecarStatus) => void): void {
    this.onStatusChange = cb;
  }

  getStatus(): SidecarStatus {
    return this.status;
  }

  getLogs(): string[] {
    return this.logs.slice(-500);
  }

  /**
   * Find the Go binary path. Search order:
   * 1. Configured path
   * 2. Packaged resources/gomikrobot
   * 3. /usr/local/bin/gomikrobot
   * 4. gomikrobot on PATH
   */
  findBinary(configuredPath?: string): string {
    if (configuredPath && fs.existsSync(configuredPath)) {
      return configuredPath;
    }

    // Dev mode: look for binary built by `make build` in project root
    // __dirname is electron/dist/main/, project root is electron/../
    const devPath = path.join(__dirname, '..', '..', '..', 'gomikrobot');
    if (fs.existsSync(devPath)) {
      return devPath;
    }

    // Packaged: look in resources directory
    const resourcesPath = path.join(process.resourcesPath || '', 'gomikrobot');
    if (fs.existsSync(resourcesPath)) {
      return resourcesPath;
    }

    // System install
    const systemPath = '/usr/local/bin/gomikrobot';
    if (fs.existsSync(systemPath)) {
      return systemPath;
    }

    // Fallback: assume on PATH
    return 'gomikrobot';
  }

  async start(mode: AppMode, configuredPath?: string): Promise<void> {
    if (this.proc) {
      return;
    }

    const binary = this.findBinary(configuredPath);
    this.setStatus('starting');
    this.logs = [];

    const env: Record<string, string> = { ...process.env as Record<string, string> };

    if (mode === 'full') {
      env['MIKROBOT_GROUP_ENABLED'] = 'true';
      env['MIKROBOT_ORCHESTRATOR_ENABLED'] = 'true';
    } else {
      env['MIKROBOT_GROUP_ENABLED'] = 'false';
    }

    // Set cwd to the binary's directory so relative paths (web/timeline.html) resolve correctly
    const binaryDir = path.dirname(path.resolve(binary));

    this.proc = spawn(binary, ['gateway'], {
      env,
      cwd: binaryDir,
      stdio: ['ignore', 'pipe', 'pipe'],
    });

    this.proc.stdout?.on('data', (data: Buffer) => {
      const line = data.toString();
      this.logs.push(line);
      if (this.logs.length > 1000) this.logs.shift();
    });

    this.proc.stderr?.on('data', (data: Buffer) => {
      const line = data.toString();
      this.logs.push(`[stderr] ${line}`);
      if (this.logs.length > 1000) this.logs.shift();
    });

    this.proc.on('exit', (code) => {
      this.proc = null;
      this.stopHealthCheck();
      if (this.status !== 'stopped') {
        this.setStatus(code === 0 ? 'stopped' : 'error');
      }
    });

    this.proc.on('error', (err) => {
      this.logs.push(`[error] ${err.message}`);
      this.setStatus('error');
    });

    // Wait for health check
    await this.waitForHealth();
    this.startHealthCheck();
  }

  async stop(): Promise<void> {
    this.stopHealthCheck();
    if (!this.proc) {
      this.setStatus('stopped');
      return;
    }

    return new Promise<void>((resolve) => {
      const forceKillTimer = setTimeout(() => {
        if (this.proc) {
          this.proc.kill('SIGKILL');
        }
        this.proc = null;
        this.setStatus('stopped');
        resolve();
      }, SHUTDOWN_TIMEOUT_MS);

      this.proc!.on('exit', () => {
        clearTimeout(forceKillTimer);
        this.proc = null;
        this.setStatus('stopped');
        resolve();
      });

      this.proc!.kill('SIGTERM');
    });
  }

  private setStatus(status: SidecarStatus): void {
    this.status = status;
    this.onStatusChange?.(status);
  }

  private async waitForHealth(): Promise<void> {
    const start = Date.now();
    while (Date.now() - start < STARTUP_TIMEOUT_MS) {
      if (await this.checkHealth()) {
        this.setStatus('running');
        return;
      }
      await new Promise((r) => setTimeout(r, HEALTH_INTERVAL_MS));
    }
    this.setStatus('error');
  }

  private startHealthCheck(): void {
    this.healthTimer = setInterval(async () => {
      const ok = await this.checkHealth();
      if (!ok && this.status === 'running') {
        this.setStatus('error');
      } else if (ok && this.status !== 'running') {
        this.setStatus('running');
      }
    }, HEALTH_INTERVAL_MS);
  }

  private stopHealthCheck(): void {
    if (this.healthTimer) {
      clearInterval(this.healthTimer);
      this.healthTimer = null;
    }
  }

  private checkHealth(): Promise<boolean> {
    return new Promise((resolve) => {
      const req = http.get(HEALTH_URL, { timeout: 3000 }, (res) => {
        resolve(res.statusCode === 200);
      });
      req.on('error', () => resolve(false));
      req.on('timeout', () => {
        req.destroy();
        resolve(false);
      });
    });
  }
}
