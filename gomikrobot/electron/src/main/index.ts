import { app, BrowserWindow, Menu, shell } from 'electron';
import * as path from 'path';
import * as fs from 'fs';
import { SidecarManager } from './sidecar';
import { RemoteClient } from './remote-client';
import { registerIpcHandlers } from './ipc-handlers';
import { resolveMode, loadElectronConfig, saveElectronConfig } from './mode-resolver';

const GATEWAY_TIMELINE_URL = 'http://127.0.0.1:18791/timeline';
const GATEWAY_GROUP_URL = 'http://127.0.0.1:18791/group';

let mainWindow: BrowserWindow | null = null;
const sidecar = new SidecarManager();
const remoteClient = new RemoteClient();

function createWindow(): BrowserWindow {
  const cfg = loadElectronConfig();
  const win = new BrowserWindow({
    width: cfg.windowState.width,
    height: cfg.windowState.height,
    x: cfg.windowState.x,
    y: cfg.windowState.y,
    title: 'GoMikroBot',
    webPreferences: {
      preload: path.join(__dirname, '..', 'preload', 'index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  // Save window state on close
  win.on('close', () => {
    const bounds = win.getBounds();
    const cfg = loadElectronConfig();
    cfg.windowState = { width: bounds.width, height: bounds.height, x: bounds.x, y: bounds.y };
    saveElectronConfig(cfg);
  });

  // Open external links in default browser
  win.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url);
    return { action: 'deny' };
  });

  return win;
}

/** Load the Vue renderer (mode picker / remote connect / settings). */
async function loadVueApp(win: BrowserWindow): Promise<void> {
  // Use Vite dev server only when explicitly requested via VITE_DEV_SERVER_URL
  const devServerUrl = process.env.VITE_DEV_SERVER_URL;
  if (devServerUrl) {
    await win.loadURL(devServerUrl);
    return;
  }

  // Otherwise load the built renderer HTML
  const rendererPath = path.join(__dirname, '..', 'renderer', 'index.html');
  if (fs.existsSync(rendererPath)) {
    await win.loadFile(rendererPath);
  } else {
    // Fallback: show helpful error instead of crashing
    await win.loadURL(`data:text/html;charset=utf-8,${encodeURIComponent(`
      <html><body style="margin:0;display:flex;align-items:center;justify-content:center;height:100vh;background:#0d1117;color:#f85149;font-family:monospace;flex-direction:column;gap:12px;">
        <div style="font-size:18px;">Renderer not found</div>
        <div style="font-size:12px;color:#8b949e;">Run <code>npm run build</code> first, or <code>npm run dev</code> for development.</div>
      </body></html>
    `)}`);
  }
}

/** Show a loading splash while sidecar starts. */
async function showLoadingScreen(win: BrowserWindow, mode: string): Promise<void> {
  const modeLabel = mode === 'full' ? 'Group Master Desktop' : 'Standalone';
  const html = `
    <html>
    <head>
      <style>
        body {
          margin: 0; display: flex; align-items: center; justify-content: center;
          height: 100vh; background: #0d1117; color: #c9d1d9;
          font-family: -apple-system, 'JetBrains Mono', monospace;
          flex-direction: column; gap: 20px;
        }
        .title { font-size: 24px; color: #58a6ff; font-weight: 600; }
        .mode  { font-size: 13px; color: #8b949e; }
        .spinner {
          width: 32px; height: 32px; border: 3px solid #30363d;
          border-top-color: #58a6ff; border-radius: 50%;
          animation: spin 0.8s linear infinite;
        }
        @keyframes spin { to { transform: rotate(360deg); } }
        .status { font-size: 12px; color: #8b949e; margin-top: 8px; }
      </style>
    </head>
    <body>
      <div class="title">GoMikroBot</div>
      <div class="mode">${modeLabel} Mode</div>
      <div class="spinner"></div>
      <div class="status">Starting gateway...</div>
    </body>
    </html>`;
  await win.loadURL(`data:text/html;charset=utf-8,${encodeURIComponent(html)}`);
}

/**
 * Start the sidecar, wait for health, then navigate to the Go-served timeline.
 * Returns true on success, false on failure.
 */
async function startSidecarAndNavigate(win: BrowserWindow, mode: 'full' | 'standalone'): Promise<boolean> {
  await showLoadingScreen(win, mode);
  const cfg = loadElectronConfig();
  try {
    await sidecar.start(mode, cfg.sidecarPath || undefined);
    if (sidecar.getStatus() === 'running') {
      await win.loadURL(GATEWAY_TIMELINE_URL);
      return true;
    }
    // Sidecar failed to become healthy — show error
    await win.loadURL(`data:text/html;charset=utf-8,${encodeURIComponent(`
      <html><body style="margin:0;display:flex;align-items:center;justify-content:center;height:100vh;background:#0d1117;color:#f85149;font-family:monospace;flex-direction:column;gap:12px;">
        <div style="font-size:18px;">Failed to start gateway</div>
        <div style="font-size:12px;color:#8b949e;">Check logs in Settings or restart the app.</div>
      </body></html>
    `)}`);
    return false;
  } catch (err: any) {
    console.error('Sidecar start failed:', err);
    return false;
  }
}

function buildMenu(): void {
  const template: Electron.MenuItemConstructorOptions[] = [
    {
      label: 'GoMikroBot',
      submenu: [
        {
          label: 'Timeline',
          accelerator: 'CmdOrCtrl+1',
          click: () => {
            if (mainWindow && sidecar.getStatus() === 'running') {
              mainWindow.loadURL(GATEWAY_TIMELINE_URL);
            }
          },
        },
        {
          label: 'Group',
          accelerator: 'CmdOrCtrl+2',
          click: () => {
            if (mainWindow && sidecar.getStatus() === 'running') {
              mainWindow.loadURL(GATEWAY_GROUP_URL);
            }
          },
        },
        { type: 'separator' },
        {
          label: 'Change Mode...',
          click: async () => {
            if (!mainWindow) return;
            const cfg = loadElectronConfig();
            cfg.mode = '';
            saveElectronConfig(cfg);
            await sidecar.stop();
            await loadVueApp(mainWindow);
          },
        },
        { type: 'separator' },
        { role: 'quit' },
      ],
    },
    {
      label: 'Edit',
      submenu: [
        { role: 'undo' },
        { role: 'redo' },
        { type: 'separator' },
        { role: 'cut' },
        { role: 'copy' },
        { role: 'paste' },
        { role: 'selectAll' },
      ],
    },
    {
      label: 'View',
      submenu: [
        { role: 'reload' },
        { role: 'forceReload' },
        { role: 'toggleDevTools' },
        { type: 'separator' },
        { role: 'zoomIn' },
        { role: 'zoomOut' },
        { role: 'resetZoom' },
        { type: 'separator' },
        { role: 'togglefullscreen' },
      ],
    },
    {
      label: 'Window',
      submenu: [
        { role: 'minimize' },
        { role: 'close' },
      ],
    },
  ];

  Menu.setApplicationMenu(Menu.buildFromTemplate(template));
}

app.whenReady().then(async () => {
  registerIpcHandlers(sidecar, remoteClient, () => mainWindow);
  buildMenu();

  // Load saved remote connections
  const cfg = loadElectronConfig();
  for (const conn of cfg.remoteConnections) {
    remoteClient.addConnection(conn);
  }

  mainWindow = createWindow();

  // Broadcast sidecar status to renderer
  sidecar.setStatusCallback((status) => {
    mainWindow?.webContents.send('sidecar:statusChanged', status);
  });

  // Resolve mode and decide what to show
  const mode = resolveMode(process.argv);

  if (mode === 'full' || mode === 'standalone') {
    // Local mode: start sidecar, show Go timeline
    await startSidecarAndNavigate(mainWindow, mode);
  } else {
    // No mode set or remote — show Vue renderer (mode picker)
    await loadVueApp(mainWindow);
  }
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('activate', async () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    mainWindow = createWindow();
    const mode = resolveMode(process.argv);
    if ((mode === 'full' || mode === 'standalone') && sidecar.getStatus() === 'running') {
      await mainWindow.loadURL(GATEWAY_TIMELINE_URL);
    } else {
      await loadVueApp(mainWindow);
    }
  }
});

app.on('before-quit', async () => {
  await sidecar.stop();
});

// Export for use by IPC handlers
export { startSidecarAndNavigate, loadVueApp, GATEWAY_TIMELINE_URL };
