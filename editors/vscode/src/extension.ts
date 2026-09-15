import * as vscode from 'vscode';
import * as path from 'path';
import { CosmClient } from './client/cosmClient';
import { CosmSCMProvider } from './scm/provider';
import { CosmStatusBar } from './views/statusBar';
import { TopologyWebviewManager } from './views/topologyWebview';
import { LineageCodeLensProvider, LineageHoverProvider } from './providers/lineageCodeLens';

let cosmStatusBar: CosmStatusBar | undefined;
let cosmSCMProvider: CosmSCMProvider | undefined;
let topologyManager: TopologyWebviewManager | undefined;

export function activate(context: vscode.ExtensionContext): void {
  const rootUri = vscode.workspace.workspaceFolders?.[0]?.uri;
  const workspaceRoot = rootUri ? rootUri.fsPath : process.cwd();

  // 1. Initialize Core Client
  const client = new CosmClient(workspaceRoot);

  // 2. Initialize SCM Provider & Status Bar
  cosmSCMProvider = new CosmSCMProvider(client, context);
  cosmStatusBar = new CosmStatusBar(client, cosmSCMProvider);
  context.subscriptions.push(cosmSCMProvider);
  context.subscriptions.push(cosmStatusBar);

  // 3. Initialize Cross-Domain Architecture Canvas Webview
  topologyManager = new TopologyWebviewManager(client);
  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider(TopologyWebviewManager.viewType, topologyManager)
  );
  context.subscriptions.push(topologyManager);

  // 4. Initialize Lineage CodeLens & Hover Providers across polyglot languages
  const polyglotSelector: vscode.DocumentSelector = [
    { language: 'go' },
    { language: 'python' },
    { language: 'typescript' },
    { language: 'typescriptreact' },
    { language: 'javascript' },
    { language: 'javascriptreact' },
    { language: 'terraform' },
    { language: 'hcl' },
    { language: 'rust' },
    { language: 'java' },
    { language: 'sql' },
    { language: 'proto' },
    { language: 'cpp' }
  ];

  const codeLensProvider = new LineageCodeLensProvider(client);
  const hoverProvider = new LineageHoverProvider(client);

  context.subscriptions.push(
    vscode.languages.registerCodeLensProvider(polyglotSelector, codeLensProvider)
  );
  context.subscriptions.push(
    vscode.languages.registerHoverProvider(polyglotSelector, hoverProvider)
  );

  // 5. Register Cosm Commands
  registerCommands(context, client, cosmSCMProvider, cosmStatusBar, topologyManager, codeLensProvider);

  // 6. Set up File Watcher on .cosm/ store to auto-refresh IDE state
  setupStoreWatcher(context, cosmSCMProvider, cosmStatusBar, topologyManager);

  // Initial update
  cosmStatusBar.update();
}

function registerCommands(
  context: vscode.ExtensionContext,
  client: CosmClient,
  scm: CosmSCMProvider,
  statusBar: CosmStatusBar,
  topology: TopologyWebviewManager,
  codeLens: LineageCodeLensProvider
): void {
  // Command: cosm.switchUniverse
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.switchUniverse', async () => {
      await statusBar.showUniversePicker();
    })
  );

  // Command: cosm.createUniverse
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.createUniverse', async () => {
      await statusBar.promptCreateUniverse();
    })
  );

  // Command: cosm.openTopology
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.openTopology', async () => {
      await topology.openEditorPanel();
    })
  );

  // Command: cosm.commitWithPrompt
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.commitWithPrompt', async () => {
      await scm.commitWithPrompt();
      codeLens.refresh();
      await topology.refresh();
    })
  );

  // Command: cosm.shipPreview
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.shipPreview', async () => {
      const config = vscode.workspace.getConfiguration('cosm');
      const targetProfile = config.get<string>('previewTarget', 'local-preview');

      vscode.window.withProgress(
        {
          location: vscode.ProgressLocation.Notification,
          title: `Cosm Ship: Compiling & projecting to '${targetProfile}'...`,
          cancellable: false
        },
        async () => {
          try {
            const res = await client.shipPreview(scm.getActiveUniverse(), targetProfile);
            const action = res.preview_url ? 'Open Preview URL' : undefined;
            const chosen = await vscode.window.showInformationMessage(
              `🚀 Shipped to '${res.target}'! Artifact: ${res.artifact_id} (${res.size_bytes} bytes). Preview: ${res.preview_url}`,
              ...(action ? [action] : [])
            );

            if (chosen === action && res.preview_url) {
              vscode.env.openExternal(vscode.Uri.parse(res.preview_url));
            }
          } catch (err: any) {
            vscode.window.showErrorMessage(`Cosm Ship failed: ${err.message}`);
          }
        }
      );
    })
  );

  // Command: cosm.blastRadius
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.blastRadius', async (targetSymbol?: string) => {
      let target = targetSymbol;
      if (!target) {
        target = await vscode.window.showInputBox({
          title: 'Cosm: Audit Blast Radius & Downstream Contracts',
          prompt: 'Enter agent ID, model name, or symbol identifier to audit',
          placeHolder: 'e.g. gemini-3.8-flash or main.HandleHealth'
        });
      }

      if (!target) return;

      try {
        const report = await client.getBlastRadius(target);
        const riskPct = (report.risk_score * 100).toFixed(0);
        vscode.window.showInformationMessage(
          `🎯 Blast Radius for '${target}': Risk Score: ${riskPct}% | Direct Nodes: ${report.TotalDirectNodes || report.total_direct_nodes} | Downstream: ${report.TotalDownstreamNodes || report.total_downstream_nodes}`
        );
      } catch (err: any) {
        vscode.window.showErrorMessage(`Blast radius failed: ${err.message}`);
      }
    })
  );

  // Command: cosm.refreshSCM
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.refreshSCM', async () => {
      await scm.refresh();
      await statusBar.update();
      await topology.refresh();
      codeLens.refresh();
      vscode.window.showInformationMessage('Cosm workspace state refreshed.');
    })
  );

  // Command: cosm.stageFile
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.stageFile', async (item) => {
      await scm.stageFile(item);
    })
  );

  // Command: cosm.unstageFile
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.unstageFile', async (item) => {
      await scm.unstageFile(item);
    })
  );

  // Command: cosm.diffSymbol
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.diffSymbol', async (fileUri, relPath) => {
      if (fileUri) {
        await scm.diffSymbol(fileUri, relPath);
      }
    })
  );

  // Command: cosm.resolveSymbol
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.resolveSymbol', async (target, relPath, identifier) => {
      if (!target) return;
      try {
        const resolved = await client.resolveSymbol(target, scm.getActiveUniverse());
        if (resolved) {
          const doc = await vscode.workspace.openTextDocument({
            language: 'json',
            content: JSON.stringify(resolved, null, 2)
          });
          await vscode.window.showTextDocument(doc, { preview: true, viewColumn: vscode.ViewColumn.Beside });
        } else {
          vscode.window.showInformationMessage(`Symbol '${target}' not found in active universe.`);
        }
      } catch (e: any) {
        vscode.window.showErrorMessage(`Error resolving symbol: ${e.message}`);
      }
    })
  );
}

function setupStoreWatcher(
  context: vscode.ExtensionContext,
  scm: CosmSCMProvider,
  statusBar: CosmStatusBar,
  topology: TopologyWebviewManager
): void {
  // Watch for changes in .cosm/ directory (e.g. CLI commits, agent edits)
  const watcher = vscode.workspace.createFileSystemWatcher('**/.cosm/**');
  let debounceTimeout: NodeJS.Timeout | undefined;

  const triggerRefresh = () => {
    if (debounceTimeout) clearTimeout(debounceTimeout);
    debounceTimeout = setTimeout(async () => {
      await scm.refresh();
      await statusBar.update();
      await topology.refresh();
    }, 400);
  };

  watcher.onDidChange(triggerRefresh);
  watcher.onDidCreate(triggerRefresh);
  watcher.onDidDelete(triggerRefresh);

  context.subscriptions.push(watcher);
}

export function deactivate(): void {
  cosmStatusBar = undefined;
  cosmSCMProvider = undefined;
  topologyManager = undefined;
}
