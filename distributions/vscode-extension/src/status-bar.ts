import * as vscode from 'vscode';
import { listenBaseUrl, telemetryBaseUrl } from './listen-url';

export type DashboardSnapshot = {
  cache_hit_rate_5m: number;
  cache_hits_5m: number;
  cache_misses_5m: number;
  compressor_bytes_saved_total: number;
  cache_replay_bytes_total: number;
  estimated_dollars_saved: number;
  injections_detected_total?: number;
  injections_blocked_total?: number;
  agent_loops_stopped_total?: number;
  budget_hits_total?: number;
  redactions_total?: number;
  recent_requests: Array<{
    route: string;
    cache_status: string;
    model?: string;
    provider?: string;
  }>;
};

const POLL_MS = 5000;
const FETCH_TIMEOUT_MS = 2500;
const IDLE_WARNING_MS = 5 * 60 * 1000; // 5 minutes

function formatBytes(n: number): string {
  if (n >= 1048576) {
    return `${(n / 1048576).toFixed(1)}MB`;
  }
  if (n >= 1024) {
    return `${(n / 1024).toFixed(1)}KB`;
  }
  return `${Math.round(n)}B`;
}

function formatTokens(bytes: number): string {
  const tokens = Math.round(bytes / 4);
  if (tokens >= 1000000) {
    return `${(tokens / 1000000).toFixed(1)}M`;
  }
  if (tokens >= 1000) {
    return `${(tokens / 1000).toFixed(1)}k`;
  }
  return `${tokens}`;
}

function formatDollars(d: number): string {
  return `$${d.toFixed(3)}`;
}

function lastCacheLabel(snapshot: DashboardSnapshot | null): string {
  const recent = snapshot?.recent_requests?.[0];
  if (!recent) {
    return 'ready';
  }

  const llmRoute =
    recent.route === '/v1/chat/completions' || recent.route === '/v1/messages';
  if (!llmRoute) {
    return 'idle';
  }

  if (recent.cache_status === 'hit' || recent.cache_status === 'miss') {
    return recent.cache_status.toUpperCase();
  }

  return 'idle';
}

function lastModel(snapshot: DashboardSnapshot | null): string {
  const recent = snapshot?.recent_requests?.[0];
  const model = recent?.model?.trim();
  if (model) {
    return model;
  }
  return recent?.provider?.trim() || '—';
}

function buildSummaryLines(snapshot: DashboardSnapshot): string[] {
  const label = lastCacheLabel(snapshot);
  const totalBytes =
    snapshot.compressor_bytes_saved_total + snapshot.cache_replay_bytes_total;
  const dollarsSaved = formatDollars(snapshot.estimated_dollars_saved);
  const tokensSaved = formatTokens(totalBytes);
  const hitRate = `${(snapshot.cache_hit_rate_5m * 100).toFixed(0)}%`;
  const hits = snapshot.cache_hits_5m || 0;
  const misses = snapshot.cache_misses_5m || 0;
  const injections = Math.round(Number(snapshot.injections_detected_total) || 0);
  const blocked = Math.round(Number(snapshot.injections_blocked_total) || 0);
  const loops = Math.round(Number(snapshot.agent_loops_stopped_total) || 0);
  const budget = Math.round(Number(snapshot.budget_hits_total) || 0);
  const redactions = Math.round(Number(snapshot.redactions_total) || 0);

  return [
    `Saved: ${dollarsSaved} (~${tokensSaved} tokens, ${formatBytes(totalBytes)})`,
    `Cache (5m): ${hitRate} · ${hits} hit / ${misses} miss`,
    `Last: ${label} · model ${lastModel(snapshot)}`,
    `Security: ${injections} inj detected` +
      (blocked > 0 ? ` (${blocked} blocked)` : '') +
      ` · ${loops} loops stopped · ${budget} budget hits`,
    `Redactions: ${redactions}`,
  ];
}

export class ProxyStatusBar implements vscode.Disposable {
  private readonly item: vscode.StatusBarItem;
  private timer: ReturnType<typeof setInterval> | undefined;
  private listenAddr: string;
  private telemetryAddr: string;
  private dashboardUrl: string;
  private running = false;
  private firstSeenAt: number | null = null;
  private hasWarnedIdle = false;
  private lastSnapshot: DashboardSnapshot | null = null;

  constructor(listenAddr: string, telemetryAddr: string) {
    this.listenAddr = listenAddr;
    this.telemetryAddr = telemetryAddr;
    this.dashboardUrl = `${telemetryBaseUrl(telemetryAddr)}/dashboard`;
    this.item = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 90);
    this.item.command = 'kotro.statusBarMenu';
    this.item.tooltip = 'Kotro Proxy Status';
    this.setOffline();
    this.item.show();
  }

  setListenAddr(listenAddr: string, telemetryAddr: string): void {
    this.listenAddr = listenAddr;
    this.telemetryAddr = telemetryAddr;
    this.dashboardUrl = `${telemetryBaseUrl(telemetryAddr)}/dashboard`;
  }

  markRunning(): void {
    this.running = true;
    if (!this.firstSeenAt) {
      this.firstSeenAt = Date.now();
      this.hasWarnedIdle = false;
    }
    void this.refresh();
    this.startPolling();
  }

  markStopped(): void {
    this.running = false;
    this.firstSeenAt = null;
    this.hasWarnedIdle = false;
    this.lastSnapshot = null;
    this.stopPolling();
    this.setOffline();
  }

  getDashboardUrl(): string {
    return this.dashboardUrl;
  }

  getLastSnapshot(): DashboardSnapshot | null {
    return this.lastSnapshot;
  }

  /** Click menu: dashboard, quick summary, Cursor Chat help, verify. */
  async showMenu(): Promise<void> {
    const snap = this.lastSnapshot;
    const dollars = snap
      ? formatDollars(snap.estimated_dollars_saved)
      : this.running
        ? '…'
        : 'offline';

    type MenuItem = vscode.QuickPickItem & { id: string };
    const items: MenuItem[] = [
      {
        id: 'dashboard',
        label: '$(graph) Open Dashboard',
        description: this.dashboardUrl,
        detail: 'Full operator UI — traffic, cache, security tiles',
      },
      {
        id: 'summary',
        label: '$(info) Quick summary',
        description: dollars === 'offline' ? 'sidecar offline' : `${dollars} saved`,
        detail: 'Show savings / hit rate / last model in a notification',
      },
      {
        id: 'verify',
        label: '$(check) Verify Cache',
        detail: 'MISS then HIT smoke test (localhost — no Cursor tunnel needed)',
      },
      {
        id: 'cursor',
        label: '$(link-external) Cursor Chat help',
        detail: 'Cursor cloud blocks localhost — tunnel / Bridge guide',
      },
      {
        id: 'logs',
        label: '$(output) Show Proxy Logs',
      },
    ];

    const pick = await vscode.window.showQuickPick(items, {
      title: 'Kotro',
      placeHolder: 'Savings glance · dashboard · Cursor Chat help',
    });
    if (!pick) {
      return;
    }

    switch (pick.id) {
      case 'dashboard':
        void vscode.commands.executeCommand('kotrolabs.openDashboard');
        break;
      case 'summary':
        await this.showQuickSummary();
        break;
      case 'verify':
        void vscode.commands.executeCommand('kotrolabs.verifyCache');
        break;
      case 'cursor':
        void vscode.commands.executeCommand('kotro.connectCursor');
        break;
      case 'logs':
        void vscode.commands.executeCommand('kotrolabs.showProxyOutput');
        break;
      default:
        break;
    }
  }

  async showQuickSummary(): Promise<void> {
    if (!this.running) {
      void vscode.window.showWarningMessage(
        'Kotro is offline. Reload the window or check Kotro: Show Proxy Logs.',
        'Show Logs',
      ).then((choice) => {
        if (choice === 'Show Logs') {
          void vscode.commands.executeCommand('kotrolabs.showProxyOutput');
        }
      });
      return;
    }

    // Refresh once so summary is current
    await this.refresh();
    const snap = this.lastSnapshot;
    if (!snap) {
      void vscode.window.showInformationMessage(
        'Kotro is running, but the dashboard API is not reachable yet.',
        'Open Dashboard',
      ).then((choice) => {
        if (choice === 'Open Dashboard') {
          void vscode.commands.executeCommand('kotrolabs.openDashboard');
        }
      });
      return;
    }

    const body = buildSummaryLines(snap).join('\n');
    const choice = await vscode.window.showInformationMessage(
      body,
      { modal: false },
      'Open Dashboard',
      'OK',
    );
    if (choice === 'Open Dashboard') {
      void vscode.commands.executeCommand('kotrolabs.openDashboard');
    }
  }

  private startPolling(): void {
    this.stopPolling();
    this.timer = setInterval(() => {
      void this.refresh();
    }, POLL_MS);
  }

  private stopPolling(): void {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = undefined;
    }
  }

  private setOffline(): void {
    this.item.text = '$(circle-slash) Kotro: offline';
    this.item.tooltip = new vscode.MarkdownString(
      'Kotro proxy sidecar is **not running**.\n\nClick for options.',
      true,
    );
  }

  private setTooltipMarkdown(lines: string[]): void {
    const md = new vscode.MarkdownString(lines.join('\n\n'), true);
    md.isTrusted = false;
    md.supportThemeIcons = true;
    this.item.tooltip = md;
  }

  private async refresh(): Promise<void> {
    if (!this.running) {
      return;
    }

    const telemetry = telemetryBaseUrl(this.telemetryAddr);
    const snapshot = await fetchDashboard(`${telemetry}/api/dashboard`);
    if (snapshot) {
      this.lastSnapshot = snapshot;
      const label = lastCacheLabel(snapshot);
      const dollarsSaved = formatDollars(snapshot.estimated_dollars_saved);
      const hitRate = `${(snapshot.cache_hit_rate_5m * 100).toFixed(0)}%`;
      const hits = snapshot.cache_hits_5m || 0;
      const misses = snapshot.cache_misses_5m || 0;

      const requests5m = hits + misses;
      const idleMs = this.firstSeenAt ? Date.now() - this.firstSeenAt : 0;

      if (requests5m === 0 && idleMs > IDLE_WARNING_MS) {
        this.item.text = `$(warning) Kotro: idle · ${dollarsSaved}`;
        this.setTooltipMarkdown([
          '$(warning) **No LLM traffic in 5m** (proxy is up)',
          'Cursor Chat cannot use `localhost` — needs HTTPS tunnel/Bridge.',
          'Continue / Cline / Claude Code / Verify Cache use localhost directly.',
          '**Click** for dashboard · summary · Cursor help',
        ]);
        if (!this.hasWarnedIdle) {
          this.hasWarnedIdle = true;
          void vscode.window
            .showWarningMessage(
              'Kotro is running, but no traffic detected. Cursor Chat needs an HTTPS tunnel (localhost is blocked).',
              'Cursor Chat help',
              'Open Dashboard',
            )
            .then((res) => {
              if (res === 'Cursor Chat help') {
                void vscode.commands.executeCommand('kotro.connectCursor');
              } else if (res === 'Open Dashboard') {
                void vscode.commands.executeCommand('kotrolabs.openDashboard');
              }
            });
        }
        return;
      }

      this.item.text = `$(pulse) Kotro: ${label} · ${dollarsSaved}`;
      const summary = buildSummaryLines(snapshot);
      this.setTooltipMarkdown([
        `$(pulse) **Kotro** · last **${label}** · ${hitRate} hit rate (5m)`,
        ...summary.map((line) => line),
        '**Click** for Open Dashboard · Quick summary · Cursor Chat help',
      ]);
      return;
    }

    this.lastSnapshot = null;
    const healthy = await probeHealth(`${listenBaseUrl(this.listenAddr)}/healthz`);
    if (healthy) {
      this.item.text = '$(sync~spin) Kotro: running';
      this.setTooltipMarkdown([
        'Proxy is **up**, but `/api/dashboard` is unavailable.',
        '**Click** for options',
      ]);
      return;
    }

    this.setOffline();
  }

  dispose(): void {
    this.stopPolling();
    this.item.dispose();
  }
}

async function fetchDashboard(url: string): Promise<DashboardSnapshot | null> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);
  try {
    const res = await fetch(url, {
      signal: controller.signal,
      headers: { 'Cache-Control': 'no-store' },
    });
    if (!res.ok) {
      return null;
    }
    return (await res.json()) as DashboardSnapshot;
  } catch {
    return null;
  } finally {
    clearTimeout(timeout);
  }
}

async function probeHealth(url: string): Promise<boolean> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);
  try {
    const res = await fetch(url, {
      signal: controller.signal,
      headers: { 'Cache-Control': 'no-store' },
    });
    return res.ok;
  } catch {
    return false;
  } finally {
    clearTimeout(timeout);
  }
}
