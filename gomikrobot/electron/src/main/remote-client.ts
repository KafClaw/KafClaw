import * as https from 'https';
import * as http from 'http';

export interface RemoteConnection {
  id: string;
  name: string;
  url: string;
  token: string;
}

export class RemoteClient {
  private connections: Map<string, RemoteConnection> = new Map();
  private activeId: string | null = null;

  addConnection(conn: RemoteConnection): void {
    this.connections.set(conn.id, conn);
  }

  removeConnection(id: string): void {
    this.connections.delete(id);
    if (this.activeId === id) this.activeId = null;
  }

  setActive(id: string): void {
    if (this.connections.has(id)) {
      this.activeId = id;
    }
  }

  getActive(): RemoteConnection | null {
    if (!this.activeId) return null;
    return this.connections.get(this.activeId) || null;
  }

  getAll(): RemoteConnection[] {
    return Array.from(this.connections.values());
  }

  /**
   * Verify connection by hitting /api/v1/status with bearer token.
   */
  async verify(conn: RemoteConnection): Promise<{ ok: boolean; error?: string; data?: any }> {
    try {
      const data = await this.fetch(conn, '/api/v1/status');
      return { ok: true, data };
    } catch (err: any) {
      return { ok: false, error: err.message };
    }
  }

  /**
   * Proxy a GET request to the remote agent.
   */
  async fetch(conn: RemoteConnection, path: string): Promise<any> {
    return new Promise((resolve, reject) => {
      const url = new URL(path, conn.url);
      const mod = url.protocol === 'https:' ? https : http;

      const req = mod.get(url.toString(), {
        headers: {
          'Authorization': `Bearer ${conn.token}`,
          'Accept': 'application/json',
        },
        timeout: 10000,
        rejectUnauthorized: false,
      }, (res) => {
        let body = '';
        res.on('data', (chunk: Buffer) => { body += chunk.toString(); });
        res.on('end', () => {
          if (res.statusCode === 401) {
            reject(new Error('Authentication failed'));
            return;
          }
          if (res.statusCode && res.statusCode >= 400) {
            reject(new Error(`HTTP ${res.statusCode}: ${body}`));
            return;
          }
          try {
            resolve(JSON.parse(body));
          } catch {
            resolve(body);
          }
        });
      });
      req.on('error', reject);
      req.on('timeout', () => {
        req.destroy();
        reject(new Error('Connection timeout'));
      });
    });
  }
}
