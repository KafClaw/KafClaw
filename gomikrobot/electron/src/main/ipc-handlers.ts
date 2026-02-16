import { BrowserWindow, ipcMain } from 'electron';
import { SidecarManager } from './sidecar';
import { RemoteClient, RemoteConnection } from './remote-client';
import {
  AppMode,
  loadElectronConfig,
  saveElectronConfig,
} from './mode-resolver';

const GATEWAY_TIMELINE_URL = 'http://127.0.0.1:18791/timeline';

export function registerIpcHandlers(
  sidecar: SidecarManager,
  remoteClient: RemoteClient,
  getMainWindow: () => BrowserWindow | null,
): void {
  // Sidecar
  ipcMain.handle('sidecar:status', () => sidecar.getStatus());
  ipcMain.handle('sidecar:logs', () => sidecar.getLogs());
  ipcMain.handle('sidecar:start', async (_event, mode: AppMode) => {
    const cfg = loadElectronConfig();
    await sidecar.start(mode, cfg.sidecarPath || undefined);
    return sidecar.getStatus();
  });
  ipcMain.handle('sidecar:stop', async () => {
    await sidecar.stop();
    return sidecar.getStatus();
  });

  // Mode
  ipcMain.handle('mode:get', () => {
    const cfg = loadElectronConfig();
    return cfg.mode;
  });
  ipcMain.handle('mode:set', (_event, mode: AppMode) => {
    const cfg = loadElectronConfig();
    cfg.mode = mode;
    saveElectronConfig(cfg);
    return mode;
  });

  /**
   * mode:activate — called from mode picker for local modes.
   * Saves the mode, starts the sidecar, waits for health,
   * then navigates the BrowserWindow to the Go-served timeline.
   */
  ipcMain.handle('mode:activate', async (_event, mode: AppMode) => {
    const cfg = loadElectronConfig();
    cfg.mode = mode;
    saveElectronConfig(cfg);

    if (mode === 'full' || mode === 'standalone') {
      const win = getMainWindow();
      if (!win) return { ok: false, error: 'No window' };

      // Show loading screen
      const modeLabel = mode === 'full' ? 'Group Master Desktop' : 'Standalone';
      const loadingHtml = `
        <html><head><style>
          body { margin:0; display:flex; align-items:center; justify-content:center;
                 height:100vh; background:#0d1117; color:#c9d1d9;
                 font-family:-apple-system,'JetBrains Mono',monospace;
                 flex-direction:column; gap:20px; }
          .title { font-size:24px; color:#58a6ff; font-weight:600; }
          .mode  { font-size:13px; color:#8b949e; }
          .spinner { width:32px; height:32px; border:3px solid #30363d;
                     border-top-color:#58a6ff; border-radius:50%;
                     animation:spin 0.8s linear infinite; }
          @keyframes spin { to { transform:rotate(360deg); } }
          .status { font-size:12px; color:#8b949e; }
        </style></head><body>
          <div class="title">GoMikroBot</div>
          <div class="mode">${modeLabel} Mode</div>
          <div class="spinner"></div>
          <div class="status">Starting gateway...</div>
        </body></html>`;
      await win.loadURL(`data:text/html;charset=utf-8,${encodeURIComponent(loadingHtml)}`);

      try {
        await sidecar.start(mode, cfg.sidecarPath || undefined);
        if (sidecar.getStatus() === 'running') {
          await win.loadURL(GATEWAY_TIMELINE_URL);
          return { ok: true };
        }
        return { ok: false, error: 'Gateway did not become healthy' };
      } catch (err: any) {
        return { ok: false, error: err.message };
      }
    }

    // Remote mode — renderer handles navigation itself
    return { ok: true };
  });

  /**
   * mode:reset — clear saved mode, stop sidecar, navigate back to Vue mode picker.
   */
  ipcMain.handle('mode:reset', async () => {
    const cfg = loadElectronConfig();
    cfg.mode = '';
    saveElectronConfig(cfg);

    const win = getMainWindow();
    if (!win) return { ok: false, error: 'No window' };

    // Import loadVueApp dynamically to avoid circular deps
    const { loadVueApp } = await import('./index');
    await sidecar.stop();
    await loadVueApp(win);
    return { ok: true };
  });

  // Config
  ipcMain.handle('config:get', () => loadElectronConfig());
  ipcMain.handle('config:save', (_event, partial: Record<string, any>) => {
    const cfg = { ...loadElectronConfig(), ...partial };
    saveElectronConfig(cfg);
    return cfg;
  });

  // Remote connections
  ipcMain.handle('remote:list', () => {
    const cfg = loadElectronConfig();
    return cfg.remoteConnections;
  });
  ipcMain.handle('remote:add', (_event, conn: RemoteConnection) => {
    const cfg = loadElectronConfig();
    cfg.remoteConnections = cfg.remoteConnections.filter((c) => c.id !== conn.id);
    cfg.remoteConnections.push(conn);
    saveElectronConfig(cfg);
    remoteClient.addConnection(conn);
    return cfg.remoteConnections;
  });
  ipcMain.handle('remote:remove', (_event, id: string) => {
    const cfg = loadElectronConfig();
    cfg.remoteConnections = cfg.remoteConnections.filter((c) => c.id !== id);
    saveElectronConfig(cfg);
    remoteClient.removeConnection(id);
    return cfg.remoteConnections;
  });
  ipcMain.handle('remote:verify', async (_event, conn: RemoteConnection) => {
    return remoteClient.verify(conn);
  });
  ipcMain.handle('remote:setActive', (_event, id: string) => {
    remoteClient.setActive(id);
    return true;
  });
  ipcMain.handle('remote:getActive', () => {
    return remoteClient.getActive();
  });

  /**
   * remote:connect — verify + navigate to remote timeline.
   */
  ipcMain.handle('remote:connect', async (_event, connId: string) => {
    remoteClient.setActive(connId);
    const conn = remoteClient.getActive();
    if (!conn) return { ok: false, error: 'Connection not found' };

    const result = await remoteClient.verify(conn);
    if (!result.ok) return { ok: false, error: result.error };

    const win = getMainWindow();
    if (win) {
      const remoteTimeline = `${conn.url.replace(/\/+$/, '')}/timeline`;
      await win.loadURL(remoteTimeline);
    }
    return { ok: true };
  });
}
