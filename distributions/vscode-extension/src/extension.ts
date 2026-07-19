import * as vscode from 'vscode';
import { spawn, ChildProcess } from 'child_process';
import * as path from 'path';
import * as fs from 'fs';
import { ProxyStatusBar } from './status-bar';
import { addrForEnv, listenBaseUrl } from './listen-url';
import { verifyCache } from './verify-cache';
import { ensureBinary } from './downloader';
import { runSetupWizard } from './setup-wizard';

let sidecarProcess: ChildProcess | null = null;
let statusBar: ProxyStatusBar | null = null;
const output = vscode.window.createOutputChannel('Kotro Proxy Engine');

function extensionConfig() {
  const cfg = vscode.workspace.getConfiguration('kotrolabs');
  return {
    profile: cfg.get<string>('profile', 'custom'),
    listenAddr: cfg.get<string>('listenAddr', ':8080'),
    metricsAddr: cfg.get<string>('metricsAddr', '127.0.0.1:9090'),
    upstreamUrl: cfg.get<string>('upstreamUrl', 'https://api.openai.com'),
    cacheDb: cfg.get<string>('cacheDb', ''),
    enableCache: cfg.get<boolean>('enableCache', true),
    enableRedaction: cfg.get<boolean>('enableRedaction', true),
    enableCompression: cfg.get<boolean>('enableCompression', true),
    enableShrink: cfg.get<boolean>('enableShrink', true),
    fallbackUrl: cfg.get<string>('fallbackUrl', ''),
    fallbackModel: cfg.get<string>('fallbackModel', ''),
    enableMetrics: cfg.get<boolean>('enableMetrics', true),
  };
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  output.appendLine('Initializing native proxy gateway core...');

  const settings = extensionConfig();
  statusBar = new ProxyStatusBar(settings.listenAddr, settings.metricsAddr);
  context.subscriptions.push(statusBar);

  context.subscriptions.push(
    vscode.commands.registerCommand('kotrolabs.openDashboard', () => {
      const url = statusBar?.getDashboardUrl() ?? 'http://127.0.0.1:9090/dashboard';
      void vscode.env.openExternal(vscode.Uri.parse(url));
    }),
  );

  context.subscriptions.push(
    vscode.commands.registerCommand('kotrolabs.showProxyOutput', () => {
      output.show(true);
    }),
  );

  context.subscriptions.push(
    vscode.commands.registerCommand('kotrolabs.verifyCache', async () => {
      output.show(true);
      output.appendLine('Running cache verification (2 identical streaming requests)...');

      const result = await verifyCache(settings.listenAddr);
      output.appendLine(result.detail);

      if (result.ok) {
        const pick = await vscode.window.showInformationMessage(
          `Kotro cache verified: ${result.detail}`,
          'Open Dashboard',
        );
        if (pick === 'Open Dashboard') {
          void vscode.commands.executeCommand('kotrolabs.openDashboard');
        }
        statusBar?.markRunning();
        return;
      }

      const pick = await vscode.window.showWarningMessage(
        `Kotro cache verification failed. ${result.detail}`,
        'Open Dashboard',
        'Show Logs',
      );
      if (pick === 'Open Dashboard') {
        void vscode.commands.executeCommand('kotrolabs.openDashboard');
      } else if (pick === 'Show Logs') {
        output.show(true);
      }
    }),
  );

  context.subscriptions.push(
    vscode.commands.registerCommand('kotro.setupWizard', async () => {
      await runSetupWizard(output);
    }),
  );

  context.subscriptions.push(
    vscode.commands.registerCommand('kotro.connectCursor', async () => {
      const pick = await vscode.window.showInformationMessage(
        'Cursor Auto mode completely bypasses local proxies. To use Kotro caching, you must configure a custom Base URL. Would you like to configure Kotro for Cursor Chat now?',
        'Yes, configure BYOK',
        'Use Continue.dev instead',
        'Learn More',
      );

      if (pick === 'Yes, configure BYOK') {
        const pick2 = await vscode.window.showInformationMessage(
          '1. Open Cursor Settings -> Models\n2. Enable "Override OpenAI Base URL" and set it to: http://localhost:8080/v1\n3. Add your API key\n4. Select a specific model (e.g. gpt-4o), do NOT use Auto.',
          'Verify Cache',
        );
        if (pick2 === 'Verify Cache') {
          void vscode.commands.executeCommand('kotrolabs.verifyCache');
        }
      } else if (pick === 'Use Continue.dev instead') {
        void vscode.commands.executeCommand('kotro.setupContinue');
      } else if (pick === 'Learn More') {
        void vscode.env.openExternal(
          vscode.Uri.parse(
            'https://github.com/kotro-labs/kotro-proxy-engine/blob/main/distributions/vscode-extension/README.md#verify-it-works-2-minutes',
          ),
        );
      }
    }),
  );

  context.subscriptions.push(
    vscode.commands.registerCommand('kotro.setupContinue', async () => {
      // Thin alias — full consent flow lives in Setup Wizard.
      await runSetupWizard(output);
    }),
  );

  const binary = await ensureBinary(context, output);

  if (!binary) {
    const msg = 'Failed to download, verify, or locate Kotro Labs binary.';
    output.appendLine(msg);
    void vscode.window.showErrorMessage(msg);
    return;
  }

  const cacheDb =
    settings.cacheDb || path.join(context.globalStorageUri.fsPath, 'kotro-cache.db');

  fs.mkdirSync(path.dirname(cacheDb), { recursive: true });

  sidecarProcess = spawn(binary.path, [], {
    env: {
      ...process.env,
      KOTRO_PROFILE: settings.profile === 'custom' ? '' : settings.profile,
      KOTRO_LISTEN_ADDR: settings.listenAddr,
      KOTRO_METRICS_ADDR: addrForEnv(settings.metricsAddr),
      KOTRO_UPSTREAM_URL: settings.upstreamUrl,
      KOTRO_CACHE_DB: cacheDb,
      KOTRO_ENABLE_CACHE: String(settings.enableCache),
      KOTRO_ENABLE_REDACTION: String(settings.enableRedaction),
      KOTRO_ENABLE_COMPRESSION: String(settings.enableCompression),
      KOTRO_ENABLE_SHRINK: String(settings.enableShrink),
      KOTRO_FALLBACK_URL: settings.fallbackUrl,
      KOTRO_FALLBACK_MODEL: settings.fallbackModel,
      KOTRO_ENABLE_METRICS: String(settings.enableMetrics),
      RUST_LOG: process.env.RUST_LOG ?? 'info',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  sidecarProcess.stdout?.on('data', (chunk: Buffer) => {
    output.appendLine(`[core] ${chunk.toString().trim()}`);
  });

  sidecarProcess.stderr?.on('data', (chunk: Buffer) => {
    output.appendLine(`[stderr] ${chunk.toString().trim()}`);
  });

  sidecarProcess.on('close', (code) => {
    output.appendLine(`Core engine exited with code ${code ?? 'unknown'}`);
    sidecarProcess = null;
    statusBar?.markStopped();
  });

  sidecarProcess.on('error', (err) => {
    output.appendLine(`Failed to start sidecar: ${err.message}`);
    void vscode.window.showErrorMessage(`Kotro Labs proxy failed to start: ${err.message}`);
    statusBar?.markStopped();
  });

  context.subscriptions.push(output);
  context.subscriptions.push({
    dispose: () => deactivate(),
  });

  statusBar.markRunning();

  const proxyBase = `${listenBaseUrl(settings.listenAddr)}/v1`;
  const runningMsg = binary.freshlyDownloaded
    ? `Binary installed and verified. Kotro proxy is running at ${proxyBase}.`
    : `Kotro proxy is running at ${proxyBase}.`;

  void vscode.window
    .showInformationMessage(
      `${runningMsg} Run Setup Wizard to configure Cline / Continue.dev?`,
      'Run Wizard',
      'Later',
      'Verify Cache',
      'Open Dashboard',
    )
    .then((pick) => {
      if (pick === 'Run Wizard') {
        void vscode.commands.executeCommand('kotro.setupWizard');
      } else if (pick === 'Verify Cache') {
        void vscode.commands.executeCommand('kotrolabs.verifyCache');
      } else if (pick === 'Open Dashboard') {
        void vscode.commands.executeCommand('kotrolabs.openDashboard');
      }
    });
}

export function deactivate(): void {
  output.appendLine('Terminating proxy sidecar process...');
  statusBar?.markStopped();
  if (!sidecarProcess) {
    return;
  }
  sidecarProcess.kill('SIGTERM');
  sidecarProcess = null;
}
