import * as vscode from 'vscode';
import * as path from 'path';
import * as fs from 'fs';
import { CosmClient } from './client/cosmClient';
import { CosmSCMProvider } from './scm/provider';
import { CosmStatusBar } from './views/statusBar';
import { TopologyWebviewManager } from './views/topologyWebview';
import {
  CosmASTTreeDataProvider,
  SymbolItem,
  DetailItem
} from './views/astTreeProvider';
import { CosmTopologyTreeDataProvider, TopologyNodeItem } from './views/topologyTreeProvider';
import { LineageCodeLensProvider, LineageHoverProvider } from './providers/lineageCodeLens';
import { CosmStackedChangesProvider, StackedChangeTreeItem } from './views/stackedChangesProvider';

let cosmStatusBar: CosmStatusBar | undefined;
let cosmSCMProvider: CosmSCMProvider | undefined;
let topologyManager: TopologyWebviewManager | undefined;
let astTreeProvider: CosmASTTreeDataProvider | undefined;
let topologyTreeProvider: CosmTopologyTreeDataProvider | undefined;
let stackedChangesProvider: CosmStackedChangesProvider | undefined;

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

  // 4. Initialize Native Architecture Topology Tree Provider
  topologyTreeProvider = new CosmTopologyTreeDataProvider(client);
  context.subscriptions.push(
    vscode.window.registerTreeDataProvider('cosm.topologyTreeView', topologyTreeProvider)
  );

  // 5. Initialize AST Merkle Tree Explorer Provider
  astTreeProvider = new CosmASTTreeDataProvider(client, cosmSCMProvider.getActiveUniverse());
  context.subscriptions.push(
    vscode.window.registerTreeDataProvider('cosm.astTreeView', astTreeProvider)
  );

  // 5. Initialize Lineage CodeLens & Hover Providers across polyglot languages
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

  // 6. Initialize Jujutsu-style Stacked Changes Provider on SCM View
  stackedChangesProvider = new CosmStackedChangesProvider(client, cosmSCMProvider);
  context.subscriptions.push(
    vscode.window.registerTreeDataProvider('cosm.scmView', stackedChangesProvider)
  );

  // 7. Register Cosm Commands
  registerCommands(
    context,
    client,
    cosmSCMProvider,
    cosmStatusBar,
    topologyManager,
    codeLensProvider,
    astTreeProvider,
    stackedChangesProvider,
    workspaceRoot
  );

  // 8. Set up File Watcher on .cosm/ store to auto-refresh IDE state
  setupStoreWatcher(
    context,
    cosmSCMProvider,
    cosmStatusBar,
    topologyManager,
    astTreeProvider,
    stackedChangesProvider
  );

  // Initial update
  cosmStatusBar.update();
}

function registerCommands(
  context: vscode.ExtensionContext,
  client: CosmClient,
  scm: CosmSCMProvider,
  statusBar: CosmStatusBar,
  topology: TopologyWebviewManager,
  codeLens: LineageCodeLensProvider,
  astTree: CosmASTTreeDataProvider,
  stackedChanges: CosmStackedChangesProvider,
  workspaceRoot: string
): void {
  async function switchMicroUniverse(target: string) {
    scm.setActiveUniverse(target);
    statusBar.setActiveUniverse(target);
    try {
      await client.switchUniverse(target);
    } catch (err) {
      // ignore if CLI switch fails
    }
    await statusBar.update();
    astTree.refresh(target);
    stackedChanges.refresh();
  }

  // Command: cosm.switchUniverse
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.switchUniverse', async (targetUniverse?: any) => {
      let target: string | undefined;
      if (typeof targetUniverse === 'string' && targetUniverse.trim()) {
        target = targetUniverse.trim();
      } else if (targetUniverse?.change && typeof targetUniverse.change.universe_id === 'string') {
        target = targetUniverse.change.universe_id.trim();
      } else if (targetUniverse?.record && typeof targetUniverse.record.universe_id === 'string') {
        target = targetUniverse.record.universe_id.trim();
      } else if (targetUniverse && typeof targetUniverse.universe_id === 'string') {
        target = targetUniverse.universe_id.trim();
      }

      if (target) {
        await switchMicroUniverse(target);
        vscode.window.showInformationMessage(`Switched to micro-universe '${target}'.`);
      } else {
        await statusBar.showUniversePicker();
      }
    })
  );

  // Command: cosm.mergeUniverse
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.mergeUniverse', async (sourceArg?: any) => {
      let sourceUniverse: string | undefined;
      if (typeof sourceArg === 'string' && sourceArg.trim()) {
        sourceUniverse = sourceArg.trim();
      } else if (sourceArg?.change && typeof sourceArg.change.universe_id === 'string') {
        sourceUniverse = sourceArg.change.universe_id.trim();
      } else if (sourceArg?.record && typeof sourceArg.record.universe_id === 'string') {
        sourceUniverse = sourceArg.record.universe_id.trim();
      } else if (sourceArg && typeof sourceArg.universe_id === 'string') {
        sourceUniverse = sourceArg.universe_id.trim();
      }

      const active = statusBar.getActiveUniverse();

      if (!sourceUniverse) {
        let universes: Array<{ universe_id: string; head_manifest_hash: string; status: string }> = [];
        try {
          universes = await client.listUniverses();
        } catch {
          // fallback
        }

        const items = universes.map(u => ({
          label: u.universe_id,
          description: `Head: ${u.head_manifest_hash ? u.head_manifest_hash.substring(0, 8) : 'root'} (${u.status})`,
          picked: u.universe_id === active && active !== 'universe-main'
        }));

        if (items.length === 0) {
          items.push({
            label: active,
            description: 'Current micro-universe',
            picked: true
          });
        } else if (active !== 'universe-main') {
          items.sort((a, b) => (a.label === active ? -1 : b.label === active ? 1 : 0));
        }

        const selectedSource = await vscode.window.showQuickPick(items, {
          title: 'Cosm: Merge Micro-Universe - Select Source',
          placeHolder: active !== 'universe-main'
            ? `Select source micro-universe to merge from (Default: ${active})`
            : 'Select source micro-universe to merge from'
        });
        if (!selectedSource) return;
        sourceUniverse = selectedSource.label;
      }

      // Prompt user for targetUniverse with QuickPick
      let allUniverses: Array<{ universe_id: string; head_manifest_hash: string; status: string }> = [];
      try {
        allUniverses = await client.listUniverses();
      } catch {
        // fallback
      }

      const targetItems: Array<{ label: string; description: string; picked?: boolean }> = [];
      const seen = new Set<string>();

      if (sourceUniverse !== 'universe-main') {
        targetItems.push({
          label: 'universe-main',
          description: 'Main production universe (Default target)',
          picked: true
        });
        seen.add('universe-main');
      }

      for (const u of allUniverses) {
        if (!seen.has(u.universe_id) && u.universe_id !== sourceUniverse) {
          targetItems.push({
            label: u.universe_id,
            description: `Head: ${u.head_manifest_hash ? u.head_manifest_hash.substring(0, 8) : 'root'} (${u.status})`,
            picked: false
          });
          seen.add(u.universe_id);
        }
      }

      if (targetItems.length === 0) {
        targetItems.push({
          label: 'universe-main',
          description: 'Main production universe',
          picked: true
        });
      }

      const selectedTarget = await vscode.window.showQuickPick(targetItems, {
        title: `Cosm: Merge '${sourceUniverse}' - Select Target Micro-Universe`,
        placeHolder: "Select target micro-universe (Default: 'universe-main')"
      });
      if (!selectedTarget) return;
      const targetUniverse = selectedTarget.label;

      // Prompt user for strategy with QuickPick
      const strategyItems = [
        { label: 'union', description: 'AST Component & Cross-Edge Union Merge (CRDT-style)', picked: true },
        { label: 'fast-forward', description: 'Fast-forward target universe head to source manifest' }
      ];

      const selectedStrategy = await vscode.window.showQuickPick(strategyItems, {
        title: `Cosm: Merge '${sourceUniverse}' into '${targetUniverse}' - Strategy`,
        placeHolder: 'Select AST merge strategy'
      });
      if (!selectedStrategy) return;
      const strategy = selectedStrategy.label;

      try {
        const result = await client.mergeUniverse(sourceUniverse, targetUniverse, strategy);
        const headShort = result.head_hash ? result.head_hash.substring(0, 12) : 'updated';
        vscode.window.showInformationMessage(
          `🔀 Merged '${sourceUniverse}' into '${targetUniverse}' (Head: ${headShort}...)`
        );
        await scm.refresh();
        await statusBar.update();
        astTree.refresh(statusBar.getActiveUniverse());
        stackedChanges.refresh();
      } catch (err: any) {
        vscode.window.showErrorMessage(`Merge universe failed: ${err.message}`);
      }
    })
  );

  // Command: cosm.showLog
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.showLog', async () => {
      const active = statusBar.getActiveUniverse();
      try {
        const entries = await client.getLog(active, 30);
        if (!entries || entries.length === 0) {
          vscode.window.showInformationMessage(`No commits found for micro-universe '${active}'.`);
          return;
        }

        const items = entries.map(entry => {
          const shortHash = entry.commit_hash ? entry.commit_hash.substring(0, 8) : 'root';
          const author = entry.author || 'unknown';
          const universeId = entry.universe_id || active;
          const intent = entry.intent || 'AST commit';
          const compCount = entry.components ? entry.components.length : 0;
          const dateStr = entry.timestamp ? new Date(entry.timestamp).toLocaleString() : '';
          const promptSuffix = entry.user_prompt ? ' | Prompt: ' + entry.user_prompt : '';

          return {
            label: `$(git-commit) ${shortHash} - ${intent}`,
            description: `(${universeId}) by ${author}`,
            detail: `${dateStr} | ${compCount} components${promptSuffix}`,
            entry
          };
        });

        await vscode.window.showQuickPick(items, {
          title: `Cosm Commit Log: ${active}`,
          placeHolder: `Cosm Commit Log for [${active}] (${entries.length} commits)`
        });
      } catch (err: any) {
        vscode.window.showErrorMessage(`Failed to retrieve commit log: ${err.message}`);
      }
    })
  );

  // Command: cosm.createUniverse
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.createUniverse', async () => {
      await statusBar.promptCreateUniverse();
      astTree.refresh(statusBar.getActiveUniverse());
      stackedChanges.refresh();
    })
  );

  // Command: cosm.openTopology
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.openTopology', async () => {
      await topology.openEditorPanel();
    })
  );

  // Command: cosm.commit
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.commit', async () => {
      await scm.commit();
      codeLens.refresh();
      await topology.refresh();
      astTree.refresh(statusBar.getActiveUniverse());
      stackedChanges.refresh();
    })
  );

  // Command: cosm.stageAll
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.stageAll', async () => {
      await scm.stageAll();
    })
  );

  // Command: cosm.unstageAll
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.unstageAll', async () => {
      await scm.unstageAll();
    })
  );

  // Command: cosm.commitWithPrompt
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.commitWithPrompt', async () => {
      await scm.commitWithPrompt();
      codeLens.refresh();
      await topology.refresh();
      astTree.refresh(statusBar.getActiveUniverse());
      stackedChanges.refresh();
    })
  );

  // Command: cosm.createStack
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.createStack', async () => {
      const changeId = await vscode.window.showInputBox({
        title: 'Cosm: Create Stacked Change (Jujutsu-style)',
        prompt: 'Enter Change ID (e.g. c/auth-jwt)',
        placeHolder: 'c/auth-jwt'
      });
      if (!changeId || !changeId.trim()) return;

      const currentUniverse = scm.getActiveUniverse();
      const defaultUniverse = currentUniverse && currentUniverse !== 'universe-main'
        ? currentUniverse
        : `u/${changeId.trim().replace(/^c\//, '')}`;

      const universeId = await vscode.window.showInputBox({
        title: 'Cosm: Create Stacked Change - Universe ID',
        prompt: 'Enter Micro-Universe ID',
        value: defaultUniverse,
        placeHolder: 'e.g. u/auth-service'
      });
      if (!universeId || !universeId.trim()) return;

      const parentId = await vscode.window.showInputBox({
        title: 'Cosm: Create Stacked Change - Parent Change',
        prompt: 'Enter parent change ID (or parent universe)',
        value: 'universe-main',
        placeHolder: 'e.g. universe-main or c/parent-change'
      });

      const title = await vscode.window.showInputBox({
        title: 'Cosm: Create Stacked Change - Title',
        prompt: 'Enter proposal title description',
        placeHolder: 'e.g. JWT Authentication Service'
      });

      try {
        await client.createStack({
          changeId: changeId.trim(),
          universeId: universeId.trim(),
          parentId: parentId && parentId.trim() ? parentId.trim() : undefined,
          title: title && title.trim() ? title.trim() : undefined
        });
        vscode.window.showInformationMessage(`🥞 Stacked change '${changeId.trim()}' created successfully.`);
        stackedChanges.refresh();
        await scm.refresh();
        await statusBar.update();
      } catch (err: any) {
        vscode.window.showErrorMessage(`Create stacked change failed: ${err.message}`);
      }
    })
  );

  // Command: cosm.evolveStack
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.evolveStack', async (item?: any) => {
      let parentChangeId: string | undefined;
      if (item && item.record && item.record.change_id) {
        parentChangeId = item.record.change_id;
      } else if (item && typeof item.change_id === 'string') {
        parentChangeId = item.change_id;
      } else if (typeof item === 'string' && item.trim()) {
        parentChangeId = item.trim();
      } else {
        const input = await vscode.window.showInputBox({
          title: 'Cosm: Evolve Stack (Auto-Rebase Descendants)',
          prompt: 'Enter parent change ID to rebase descendants onto',
          placeHolder: 'e.g. c/auth-jwt or universe-main'
        });
        if (input && input.trim()) {
          parentChangeId = input.trim();
        }
      }

      if (!parentChangeId) return;

      try {
        const res = await client.evolveStack(parentChangeId.trim());
        vscode.window.showInformationMessage(`⚡ Evolved stack on '${parentChangeId.trim()}'. ${res}`);
        stackedChanges.refresh();
        await scm.refresh();
        await statusBar.update();
      } catch (err: any) {
        vscode.window.showErrorMessage(`Evolve stack failed: ${err.message}`);
      }
    })
  );

  // Command: cosm.refreshStack
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.refreshStack', () => {
      stackedChanges.refresh();
      vscode.window.showInformationMessage('Cosm Stacked Proposals refreshed.');
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
    vscode.commands.registerCommand('cosm.blastRadius', async (targetSymbol?: any) => {
      let target: string | undefined;
      if (targetSymbol instanceof SymbolItem) {
        target = targetSymbol.symbol.identifier || targetSymbol.symbol.node_id;
      } else if (targetSymbol instanceof TopologyNodeItem) {
        target = targetSymbol.node.id || targetSymbol.node.name;
      } else if (typeof targetSymbol === 'string') {
        target = targetSymbol;
      } else if (targetSymbol?.symbol) {
        target = targetSymbol.symbol.identifier || targetSymbol.symbol.node_id;
      } else if (targetSymbol?.node) {
        target = targetSymbol.node.id || targetSymbol.node.name;
      }

      if (!target) {
        target = await vscode.window.showInputBox({
          title: 'Cosm: Audit Blast Radius & Downstream Contracts',
          prompt: 'Enter agent ID, model name, or symbol identifier to audit',
          placeHolder: 'e.g. gemini-3.8-flash or main.HandleHealth'
        });
      }

      if (!target) {
        return;
      }

      try {
        const report = await client.getBlastRadius(target);
        const riskPct = (report.risk_score * 100).toFixed(0);
        const direct = report.total_direct_nodes ?? report.TotalDirectNodes ?? 0;
        const downstream = report.total_downstream_nodes ?? report.TotalDownstreamNodes ?? 0;
        const warningCount = report.warnings ? report.warnings.length : 0;
        const warningSuffix = warningCount > 0 ? ` | ⚠️ ${warningCount} warnings` : '';

        const msg = `🎯 Blast Radius for '${target}': Risk Score: ${riskPct}% | Direct Contracts: ${direct} | Downstream: ${downstream}${warningSuffix}`;

        if (warningCount > 0 || (report.affected_components && report.affected_components.length > 0)) {
          vscode.window.showInformationMessage(msg, 'View Details').then((selection: string | undefined) => {
            if (selection === 'View Details') {
              const lines = [
                `# Blast Radius Assessment: \`${target}\``,
                '',
                `**Risk Score**: ${riskPct}% | **Direct Contracts**: ${direct} | **Downstream Components**: ${downstream}`,
                '',
                `## Summary`,
                `${report.summary || 'Impact cascade analysis based on AST Merkle DAG dependency edges.'}`,
                '',
                `## Incoming Edges (${report.incoming_edges?.length || 0})`,
                ...(report.incoming_edges?.map(e => `- \`${e}\``) || ['- None']),
                '',
                `## Outgoing Edges (${report.outgoing_edges?.length || 0})`,
                ...(report.outgoing_edges?.map(e => `- \`${e}\``) || ['- None']),
                '',
                `## Affected Components (${report.affected_components?.length || 0})`,
                ...(report.affected_components?.map(c => `- \`${c}\``) || ['- None']),
                '',
                `## Warnings (${warningCount})`,
                ...(report.warnings?.map(w => `- ⚠️ ${w}`) || ['- None'])
              ];
              vscode.workspace.openTextDocument({
                content: lines.join('\n'),
                language: 'markdown'
              }).then((doc: vscode.TextDocument) => {
                vscode.window.showTextDocument(doc, { preview: true, viewColumn: vscode.ViewColumn.Beside });
              });
            }
          });
        } else {
          vscode.window.showInformationMessage(msg);
        }
      } catch (err: any) {
        vscode.window.showErrorMessage(`Blast radius failed: ${err.message}`);
      }
    })
  );

  // Command: cosm.explainMerkleNodes
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.explainMerkleNodes', async () => {
      const content = [
        '# Cosm: Understanding the AST Merkle-DAG & Cryptographic Nodes',
        '',
        'In traditional source control systems like Git, code is stored as line-based text diffs and file snapshots. **Cosm (`cosm`) fundamentally changes this paradigm** by representing software as a **polyglot, content-addressed AST Merkle Directed Acyclic Graph (DAG)**.',
        '',
        '---',
        '',
        '## 1. What is an AST Merkle Node?',
        '',
        'Every structural element of your code—functions, structs, classes, API routes, database models, and cloud resources—is parsed into an immutable **`ASTSymbolNode`**.',
        '',
        '- **Deterministic SHA-256 Content Addressing**: Each symbol\'s `node_id` is computed by hashing its normalized syntax payload and outgoing dependency contracts.',
        '- **Language-Agnostic Semantics**: Whether written in Go, TypeScript, Python, Rust, or Terraform HCL, nodes express uniform semantic types (`FunctionDecl`, `StructDecl`, `ResourceBlock`).',
        '- **Zero-Tamper Immutability**: If a single character or parameter within a function changes, its SHA-256 hash changes. Unchanged symbols retain identical hashes.',
        '',
        '---',
        '',
        '## 2. Merkle Tree Folding (From Symbol to Universe Root)',
        '',
        'Cosm organizes software hierarchically using bottom-up cryptographic folding:',
        '',
        '```',
        '                     🌌 Universe Merkle Root',
        '                   (WorkspaceManifestNode SHA-256)',
        '                             /          \\',
        '                            /            \\',
        '               📦 ComponentNode       📦 ComponentNode',
        '               (cmd/server/main.go)   (pkg/storage/store.go)',
        '                    /        \\              /        \\',
        '                   /          \\            /          \\',
        '            💎 ASTSymbol  💎 ASTSymbol 💎 ASTSymbol 💎 ASTSymbol',
        '             (Server)   (MemoryStore)  (PutEdge)     (GetBlob)',
        '```',
        '',
        '1. **AST Symbol Nodes**: Individual functions and structs (leaves of the tree).',
        '2. **Component Merkle Nodes**: Files aggregating child symbol hashes.',
        '3. **Universe Merkle Root**: The top-level cryptographic hash of the entire active universe.',
        '',
        '---',
        '',
        '## 3. Causal AI Pedigree (Digital Attestations)',
        '',
        'Every Merkle node can carry an immutable cryptographic lineage envelope signed with **Ed25519**:',
        '',
        '$$\\text{User Prompt} \\longrightarrow \\text{Session ID} \\longrightarrow \\text{Agent ID} \\longrightarrow \\text{Model Params} \\longrightarrow \\text{AST Node}$$',
        '',
        '- Know exactly which prompt and AI agent created or edited every function.',
        '- Verify cryptographic signatures to prevent unauthorized code injection.',
        '- Trace changes across model generations (`gemini-3.8-flash`, human review).',
        '',
        '---',
        '',
        '## 4. Zero-Copy Parallel Micro-Universes',
        '',
        'Because code is stored as an immutable Merkle-DAG:',
        '- Creating a branch (**Micro-Universe**) is instant ($O(1)$) and consumes **zero disk space**.',
        '- Universes only record divergent Merkle nodes; unchanged components share identical hashes.',
        '- Agents and developers work in isolated micro-universes with zero blocking locks.',
        '',
        '---',
        '',
        '*Powered by Cosm AST Engine (`cosm`) & Topocosm Hub (`topocosm.dev`)*'
      ].join('\n');

      const doc = await vscode.workspace.openTextDocument({
        content,
        language: 'markdown'
      });
      await vscode.window.showTextDocument(doc, { preview: true, viewColumn: vscode.ViewColumn.Beside });
    })
  );

  // Command: cosm.refreshTopologyTree
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.refreshTopologyTree', async () => {
      topologyTreeProvider?.refresh();
      vscode.window.showInformationMessage('Architecture Topology refreshed.');
    })
  );

  // Command: cosm.refreshSCM
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.refreshSCM', async () => {
      await scm.refresh();
      await statusBar.update();
      await topology.refresh();
      topologyTreeProvider?.refresh();
      codeLens.refresh();
      astTree.refresh(statusBar.getActiveUniverse());
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
      if (fileUri instanceof SymbolItem) {
        const targetUri = vscode.Uri.file(path.resolve(workspaceRoot, fileUri.filePath));
        await scm.diffSymbol(targetUri, fileUri.filePath);
        return;
      }
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

  // Command: cosm.refreshASTTree
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.refreshASTTree', async () => {
      astTree.refresh(statusBar.getActiveUniverse());
      vscode.window.showInformationMessage('Cosm AST Merkle Tree refreshed.');
    })
  );

  // Command: cosm.navigateToSymbol
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.navigateToSymbol', async (filePathOrItem: any, symbolArg?: any) => {
      let filePath: string | undefined;
      let symbol: any;

      if (filePathOrItem instanceof SymbolItem) {
        filePath = filePathOrItem.filePath;
        symbol = filePathOrItem.symbol;
      } else if (filePathOrItem instanceof TopologyNodeItem) {
        filePath = filePathOrItem.node.file_path || filePathOrItem.node.name;
        symbol = filePathOrItem.node.name;
      } else if (typeof filePathOrItem === 'string') {
        filePath = filePathOrItem;
        symbol = symbolArg;
      } else if (filePathOrItem && typeof filePathOrItem === 'object') {
        filePath = filePathOrItem.file_path || filePathOrItem.filePath || filePathOrItem.component_id || filePathOrItem.name;
        symbol = filePathOrItem.symbol || filePathOrItem.name || filePathOrItem;
      }

      if (!filePath && !symbol) {
        vscode.window.showWarningMessage('No file path or symbol provided for AST navigation.');
        return;
      }

      let absPath = filePath
        ? (path.isAbsolute(filePath) ? filePath : path.resolve(workspaceRoot, filePath))
        : '';

      // Multi-tier file resolution if file does not exist directly
      if (!absPath || !fs.existsSync(absPath)) {
        // Tier 1: Search by relative path suffix or filename in workspace
        if (filePath) {
          let matches = await vscode.workspace.findFiles(`**/${filePath}`, '**/node_modules/**', 1);
          if (matches.length === 0) {
            const baseName = path.basename(filePath);
            if (baseName && !baseName.includes(':')) {
              matches = await vscode.workspace.findFiles(`**/${baseName}`, '**/node_modules/**', 1);
            }
          }
          if (matches.length > 0) {
            absPath = matches[0].fsPath;
          }
        }

        // Tier 2: If absPath still doesn't exist, filePath might be a symbol (e.g. main.MemoryStore)
        if (!absPath || !fs.existsSync(absPath)) {
          const targetSym = typeof symbol === 'string'
            ? symbol
            : (symbol?.identifier || filePath || '');
          const cleanName = targetSym.includes('.') ? targetSym.split('.').pop()! : targetSym;

          if (cleanName) {
            const files = await vscode.workspace.findFiles('**/*.{go,ts,tsx,js,jsx,py,hcl,tf,rs,zig,sql}', '**/node_modules/**', 50);
            for (const file of files) {
              try {
                const text = fs.readFileSync(file.fsPath, 'utf8');
                if (text.includes(cleanName)) {
                  absPath = file.fsPath;
                  if (!symbol || typeof symbol === 'string') {
                    symbol = cleanName;
                  }
                  break;
                }
              } catch {}
            }
          }
        }
      }

      if (!absPath || !fs.existsSync(absPath)) {
        vscode.window.showWarningMessage(`Could not resolve file on disk for: ${filePath || symbol}`);
        return;
      }

      try {
        const doc = await vscode.workspace.openTextDocument(vscode.Uri.file(absPath));
        const editor = await vscode.window.showTextDocument(doc, { preview: false });

        const identifier = typeof symbol === 'string' ? symbol : symbol?.identifier || '';
        const signature = typeof symbol === 'object' ? symbol?.signature : undefined;

        if (!identifier && !signature) {
          return;
        }

        const cleanIdent = identifier.includes('.') ? identifier.split('.').pop()! : identifier;

        let matchedLine = -1;
        let matchedChar = 0;
        let matchLength = cleanIdent ? cleanIdent.length : 1;

        // Pass 1: Match full or partial signature if available
        if (signature) {
          const sigTrimmed = signature.trim();
          for (let i = 0; i < doc.lineCount; i++) {
            const text = doc.lineAt(i).text;
            if (text.includes(sigTrimmed)) {
              matchedLine = i;
              matchedChar = text.indexOf(sigTrimmed);
              matchLength = sigTrimmed.length;
              break;
            }
          }
        }

        // Pass 1.5: Match Terraform / HCL blocks and SQL statements
        if (matchedLine === -1 && identifier) {
          if (identifier.startsWith('resource.')) {
            const parts = identifier.split('.');
            if (parts.length >= 3) {
              const resType = parts[1];
              const resName = parts[2];
              const tfRegex = new RegExp(`resource\\s+"${resType}"\\s+"${resName}"`);
              for (let i = 0; i < doc.lineCount; i++) {
                const text = doc.lineAt(i).text;
                if (tfRegex.test(text)) {
                  matchedLine = i;
                  matchedChar = text.indexOf(resName);
                  matchLength = resName.length;
                  break;
                }
              }
            }
          } else if (identifier.startsWith('variable.') || identifier.startsWith('output.') || identifier.startsWith('provider.')) {
            const parts = identifier.split('.');
            if (parts.length >= 2) {
              const blockType = parts[0];
              const blockName = parts[1];
              const tfRegex = new RegExp(`${blockType}\\s+"${blockName}"`);
              for (let i = 0; i < doc.lineCount; i++) {
                const text = doc.lineAt(i).text;
                if (tfRegex.test(text)) {
                  matchedLine = i;
                  matchedChar = text.indexOf(blockName);
                  matchLength = blockName.length;
                  break;
                }
              }
            }
          } else if (identifier.startsWith('table:')) {
            const tableName = identifier.split(':').pop()!;
            const sqlRegex = new RegExp(`CREATE\\s+TABLE(?:\\s+IF\\s+NOT\\s+EXISTS)?\\s+${tableName}\\b`, 'i');
            for (let i = 0; i < doc.lineCount; i++) {
              const text = doc.lineAt(i).text;
              if (sqlRegex.test(text)) {
                matchedLine = i;
                matchedChar = text.toLowerCase().indexOf(tableName.toLowerCase());
                matchLength = tableName.length;
                break;
              }
            }
          }
        }

        // Pass 2: Look for language declaration keywords containing the clean identifier or full identifier
        if (matchedLine === -1 && (cleanIdent || identifier)) {
          const identsToTry = cleanIdent !== identifier ? [cleanIdent, identifier] : [cleanIdent];
          for (const targetId of identsToTry) {
            const escaped = targetId.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
            const declRegex = new RegExp(`\\b(func|def|class|type|interface|const|var|let|export|struct|fn|resource)\\s+${escaped}\\b`);
            for (let i = 0; i < doc.lineCount; i++) {
              const text = doc.lineAt(i).text;
              if (declRegex.test(text)) {
                matchedLine = i;
                matchedChar = text.indexOf(targetId);
                matchLength = targetId.length;
                break;
              }
            }
            if (matchedLine !== -1) break;
          }
        }

        // Pass 3: Word boundary match on clean identifier
        if (matchedLine === -1 && cleanIdent) {
          const escaped = cleanIdent.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
          const wordRegex = new RegExp(`\\b${escaped}\\b`);
          for (let i = 0; i < doc.lineCount; i++) {
            const text = doc.lineAt(i).text;
            if (wordRegex.test(text)) {
              matchedLine = i;
              matchedChar = text.indexOf(cleanIdent);
              matchLength = cleanIdent.length;
              break;
            }
          }
        }

        // Pass 4: Fallback substring search
        if (matchedLine === -1 && cleanIdent) {
          for (let i = 0; i < doc.lineCount; i++) {
            const text = doc.lineAt(i).text;
            const idx = text.indexOf(cleanIdent);
            if (idx !== -1) {
              matchedLine = i;
              matchedChar = idx;
              matchLength = cleanIdent.length;
              break;
            }
          }
        }

        if (matchedLine !== -1) {
          const startPos = new vscode.Position(matchedLine, Math.max(0, matchedChar));
          const endPos = new vscode.Position(matchedLine, Math.max(0, matchedChar + matchLength));
          const range = new vscode.Range(startPos, endPos);
          editor.selection = new vscode.Selection(startPos, endPos);
          editor.revealRange(range, vscode.TextEditorRevealType.InCenter);
        }
      } catch (err: any) {
        vscode.window.showErrorMessage(`Unable to open ${filePath}: ${err.message}`);
      }
    })
  );

  // Command: cosm.viewASTPayload
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.viewASTPayload', async (item: any) => {
      let nodeId: string | undefined;
      let identifier: string | undefined;
      let filePath: string | undefined;

      if (item instanceof SymbolItem) {
        nodeId = item.symbol.node_id;
        identifier = item.symbol.identifier;
        filePath = item.filePath;
      } else if (item instanceof DetailItem) {
        nodeId = item.value;
      } else if (typeof item === 'string') {
        nodeId = item;
      } else if (item && typeof item === 'object') {
        nodeId = item.node_id || item.symbol?.node_id;
        identifier = item.identifier || item.symbol?.identifier;
        filePath = item.filePath;
      }

      if (!nodeId && !identifier) {
        vscode.window.showWarningMessage('No AST symbol or node ID provided to view payload.');
        return;
      }

      let payload = '';

      // 1. Try viewing raw or AST node via client.viewNode
      if (nodeId) {
        try {
          payload = await client.viewNode(nodeId, 'ast');
        } catch {
          // Fall back to resolved symbol
        }
      }

      // 2. Try client.resolveSymbol
      if (!payload) {
        const target = filePath && identifier ? `${filePath}:${identifier}` : (identifier || nodeId || '');
        try {
          const resolved = await client.resolveSymbol(target, scm.getActiveUniverse());
          if (resolved) {
            payload = JSON.stringify(resolved, null, 2);
          }
        } catch {
          // Fall back
        }
      }

      // 3. Try client.viewNode raw format
      if (!payload && nodeId) {
        try {
          payload = await client.viewNode(nodeId, 'raw');
        } catch {
          // Fall back
        }
      }

      // 4. Fall back to symbol object if available
      if (!payload) {
        if (item instanceof SymbolItem) {
          payload = JSON.stringify(item.symbol, null, 2);
        } else if (item?.symbol) {
          payload = JSON.stringify(item.symbol, null, 2);
        } else {
          payload = JSON.stringify({ node_id: nodeId, identifier: identifier || '' }, null, 2);
        }
      }

      const isJson = payload.trim().startsWith('{') || payload.trim().startsWith('[');
      const doc = await vscode.workspace.openTextDocument({
        language: isJson ? 'json' : 'yaml',
        content: payload
      });
      await vscode.window.showTextDocument(doc, {
        preview: true,
        viewColumn: vscode.ViewColumn.Beside
      });
    })
  );

  // Command: cosm.copyNodeId
  context.subscriptions.push(
    vscode.commands.registerCommand('cosm.copyNodeId', async (item: any) => {
      let nodeId: string | undefined;

      if (item instanceof SymbolItem) {
        nodeId = item.symbol.node_id;
      } else if (item instanceof DetailItem) {
        nodeId = item.value || (item.description ? String(item.description) : undefined);
      } else if (typeof item === 'string') {
        nodeId = item;
      } else if (item && typeof item === 'object') {
        nodeId = item.node_id || item.symbol?.node_id || item.value;
      }

      if (!nodeId) {
        vscode.window.showWarningMessage('No node ID available to copy.');
        return;
      }

      await vscode.env.clipboard.writeText(nodeId);
      vscode.window.showInformationMessage(`Copied AST Node ID: ${nodeId}`);
    })
  );
}

function setupStoreWatcher(
  context: vscode.ExtensionContext,
  scm: CosmSCMProvider,
  statusBar: CosmStatusBar,
  topology: TopologyWebviewManager,
  astTree: CosmASTTreeDataProvider,
  stackedChanges: CosmStackedChangesProvider
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
      astTree.refresh(statusBar.getActiveUniverse());
      stackedChanges.refresh();
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
  astTreeProvider = undefined;
  stackedChangesProvider = undefined;
}
