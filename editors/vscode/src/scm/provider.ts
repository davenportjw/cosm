import * as vscode from 'vscode';
import * as path from 'path';
import * as fs from 'fs';
import { CosmClient } from '../client/cosmClient';
import { CosmStatus } from '../types';

export class CosmContentProvider implements vscode.TextDocumentContentProvider {
  static scheme = 'cosm';
  private client: CosmClient;
  private onDidChangeEmitter = new vscode.EventEmitter<vscode.Uri>();
  public readonly onDidChange = this.onDidChangeEmitter.event;

  constructor(client: CosmClient) {
    this.client = client;
  }

  public async provideTextDocumentContent(uri: vscode.Uri): Promise<string> {
    // uri: cosm://universe-head/path/to/file or cosm://node/<node_id>
    try {
      if (uri.authority === 'node') {
        const nodeId = uri.path.replace(/^\//, '');
        return await this.client.viewNode(nodeId, 'source');
      }

      // Check if viewing symbol via query param
      const query = new URLSearchParams(uri.query);
      const symbolTarget = query.get('symbol');
      if (symbolTarget) {
        const resolved = await this.client.resolveSymbol(symbolTarget);
        if (resolved && resolved.symbol_node.ast_payload) {
          return resolved.symbol_node.ast_payload;
        }
      }

      // Default fallback
      return `// Cosm Universe Head: ${uri.authority}\n// Node: ${uri.path}\n`;
    } catch (err: any) {
      return `// Failed to load Cosm AST content: ${err.message}`;
    }
  }

  public update(uri: vscode.Uri): void {
    this.onDidChangeEmitter.fire(uri);
  }
}

export class CosmSCMProvider implements vscode.Disposable {
  private scm: vscode.SourceControl;
  private stagedGroup: vscode.SourceControlResourceGroup;
  private modifiedGroup: vscode.SourceControlResourceGroup;
  private client: CosmClient;
  private activeUniverse: string = 'universe-main';
  private disposables: vscode.Disposable[] = [];

  constructor(client: CosmClient, context: vscode.ExtensionContext) {
    this.client = client;

    const rootUri = vscode.Uri.file(client.getWorkspaceRoot());
    this.scm = vscode.scm.createSourceControl('cosm', 'Cosm AST', rootUri);
    this.scm.inputBox.placeholder = 'Commit Intent (e.g. feat(auth): add token caching)';
    this.scm.actionButton = {
      command: { command: 'cosm.commit', title: '✓ Commit AST Changes' },
      enabled: true
    };
    this.scm.acceptInputCommand = {
      command: 'cosm.commit',
      title: 'Commit AST Changes'
    };

    this.stagedGroup = this.scm.createResourceGroup('staged', 'Staged AST Components');
    this.modifiedGroup = this.scm.createResourceGroup('modified', 'Working Tree AST Symbols');

    this.stagedGroup.hideWhenEmpty = true;
    this.modifiedGroup.hideWhenEmpty = false;

    // Register virtual content provider for AST diffing
    const contentProvider = new CosmContentProvider(this.client);
    this.disposables.push(
      vscode.workspace.registerTextDocumentContentProvider(CosmContentProvider.scheme, contentProvider)
    );

    this.disposables.push(this.scm);
    this.refresh();
  }

  public getActiveUniverse(): string {
    return this.activeUniverse;
  }

  public setActiveUniverse(universeId: string): void {
    this.activeUniverse = universeId;
    void this.refresh();
  }

  /**
   * Refreshes SCM status from the Cosm CLI.
   */
  public async refresh(): Promise<CosmStatus> {
    try {
      const status = await this.client.getStatus(this.activeUniverse);
      this.activeUniverse = status.universe_id;
      this.scm.statusBarCommands = [
        {
          command: 'cosm.switchUniverse',
          title: `$(globe) [🌌 ${this.activeUniverse}]`,
          tooltip: `Cosm Micro-Universe: ${this.activeUniverse}\nMerkle Head: ${status.merkle_root.substring(0, 12)}...\nComponents: ${status.components_count}\nCross-Boundary Edges: ${status.cross_edges_count}`
        }
      ];

      // Inspect workspace files vs Cosm tracked components
      await this.scanWorkingTree(status);
      return status;
    } catch (err: any) {
      this.scm.statusBarCommands = [
        {
          command: 'cosm.switchUniverse',
          title: `$(alert) Cosm [${this.activeUniverse}]`,
          tooltip: `Cosm status error: ${err.message}`
        }
      ];
      return {
        status: 'ERROR',
        universe_id: this.activeUniverse,
        merkle_root: '',
        components_count: 0,
        cross_edges_count: 0,
        components: []
      };
    }
  }

  /**
   * Inspects working tree files and organizes them into staged and modified groups.
   */
  private async scanWorkingTree(status: CosmStatus): Promise<void> {
    const rootPath = this.client.getWorkspaceRoot();
    const stagedResources: vscode.SourceControlResourceState[] = [];
    const modifiedResources: vscode.SourceControlResourceState[] = [];
    const seenPaths = new Set<string>();

    // 1. Process explicitly staged files
    if (status.staged_files && Array.isArray(status.staged_files)) {
      for (const entry of status.staged_files) {
        const cleanPath = entry.includes(' (') ? entry.split(' (')[0].trim() : entry.trim();
        if (!cleanPath || seenPaths.has(cleanPath)) continue;
        seenPaths.add(cleanPath);

        const fullPath = path.isAbsolute(cleanPath) ? cleanPath : path.resolve(rootPath, cleanPath);
        const fileUri = vscode.Uri.file(fullPath);
        stagedResources.push({
          resourceUri: fileUri,
          decorations: {
            strikeThrough: false,
            tooltip: `Staged AST Component: ${cleanPath}`
          },
          command: {
            command: 'cosm.diffSymbol',
            title: 'Diff AST Symbol',
            arguments: [fileUri, cleanPath]
          }
        });
      }
    }

    // 2. Process drifted / modified files reported by working_tree_lens
    const driftedList = status.working_tree_lens?.drifted_files || status.modified_files || [];
    for (const entry of driftedList) {
      const cleanPath = entry.includes(' (') ? entry.split(' (')[0].trim() : entry.trim();
      if (!cleanPath || seenPaths.has(cleanPath)) continue;
      seenPaths.add(cleanPath);

      const fullPath = path.isAbsolute(cleanPath) ? cleanPath : path.resolve(rootPath, cleanPath);
      const fileUri = vscode.Uri.file(fullPath);
      const isTracked = status.components.includes(cleanPath) || status.components.includes(path.basename(cleanPath));
      modifiedResources.push({
        resourceUri: fileUri,
        decorations: {
          strikeThrough: false,
          tooltip: `${isTracked ? 'Modified' : 'Untracked'} AST Component: ${cleanPath}`
        },
        command: {
          command: 'cosm.diffSymbol',
          title: 'Diff AST Symbol',
          arguments: [fileUri, cleanPath]
        }
      });
    }

    // 3. Polyglot extensions supported by Cosm codecs - inspect directory for untracked components
    const targetExtensions = ['.go', '.py', '.ts', '.tsx', '.tf', '.rs', '.java', '.sql', '.proto', '.cpp'];

    const inspectDir = (dir: string, depth = 0) => {
      if (depth > 4) return;
      try {
        const entries = fs.readdirSync(dir, { withFileTypes: true });
        for (const entry of entries) {
          if (entry.name.startsWith('.') || entry.name === 'node_modules' || entry.name === 'out' || entry.name === 'bin') {
            continue;
          }
          const fullPath = path.join(dir, entry.name);
          if (entry.isDirectory()) {
            inspectDir(fullPath, depth + 1);
          } else if (entry.isFile()) {
            const ext = path.extname(entry.name);
            if (targetExtensions.includes(ext)) {
              const relPath = path.relative(rootPath, fullPath);
              if (seenPaths.has(relPath)) continue;
              seenPaths.add(relPath);

              const fileUri = vscode.Uri.file(fullPath);
              const isTracked = status.components.includes(relPath) || status.components.includes(entry.name);
              const resourceState: vscode.SourceControlResourceState = {
                resourceUri: fileUri,
                decorations: {
                  strikeThrough: false,
                  tooltip: `${isTracked ? 'Tracked' : 'Untracked'} AST Component: ${relPath}`
                },
                command: {
                  command: 'cosm.diffSymbol',
                  title: 'Diff AST Symbol',
                  arguments: [fileUri, relPath]
                }
              };

              if (isTracked || modifiedResources.length < 30) {
                modifiedResources.push(resourceState);
              }
            }
          }
        }
      } catch {
        // ignore read errors
      }
    };

    inspectDir(rootPath);

    this.stagedGroup.resourceStates = stagedResources;
    this.modifiedGroup.resourceStates = modifiedResources;
  }

  /**
   * Stage an AST component file, resource group, or all modified files into the Cosm store.
   */
  public async stageFile(resource?: vscode.SourceControlResourceState | vscode.SourceControlResourceGroup | vscode.Uri | any): Promise<void> {
    if (!resource) {
      await this.stageAll();
      return;
    }

    try {
      // Check if resource is a SourceControlResourceGroup (e.g. user clicked '+' on modified group)
      if (resource.id === 'modified' || Array.isArray(resource.resourceStates)) {
        const states: vscode.SourceControlResourceState[] = resource.resourceStates || [];
        if (states.length === 0) {
          await this.stageAll();
          return;
        }
        const filesToStage = states.map(s => path.relative(this.client.getWorkspaceRoot(), s.resourceUri.fsPath));
        await this.client.stageFiles(filesToStage, undefined, undefined, this.activeUniverse);
        vscode.window.showInformationMessage(`✓ Staged ${filesToStage.length} AST Component(s)`);
        await this.refresh();
        return;
      }

      // Check if resource is SourceControlResourceState or vscode.Uri
      let targetPath: string | undefined;
      if (resource.resourceUri && typeof resource.resourceUri.fsPath === 'string') {
        targetPath = resource.resourceUri.fsPath;
      } else if (typeof resource.fsPath === 'string') {
        targetPath = resource.fsPath;
      }

      if (targetPath) {
        const relPath = path.relative(this.client.getWorkspaceRoot(), targetPath);
        await this.client.stageFiles([relPath], undefined, undefined, this.activeUniverse);
        vscode.window.showInformationMessage(`✓ Staged AST Component: ${path.basename(relPath)}`);
        await this.refresh();
        return;
      }

      await this.stageAll();
    } catch (err: any) {
      vscode.window.showErrorMessage(`Staging failed: ${err.message}`);
    }
  }

  /**
   * Stages all modified files or the entire workspace into the Cosm store.
   */
  public async stageAll(): Promise<boolean> {
    try {
      const files: string[] = [];
      if (this.modifiedGroup.resourceStates && Array.isArray(this.modifiedGroup.resourceStates)) {
        for (const r of this.modifiedGroup.resourceStates) {
          if (r?.resourceUri?.fsPath) {
            const rel = path.relative(this.client.getWorkspaceRoot(), r.resourceUri.fsPath);
            if (rel && !files.includes(rel)) {
              files.push(rel);
            }
          }
        }
      }
      await this.client.stageFiles(files.length > 0 ? files : ['.'], undefined, undefined, this.activeUniverse);
      vscode.window.showInformationMessage('✓ Staged all AST components');
      await this.refresh();
      return true;
    } catch (err: any) {
      vscode.window.showErrorMessage(`Staging all failed: ${err.message}`);
      return false;
    }
  }

  /**
   * Unstages an AST component or all staged components.
   */
  public async unstageFile(resource?: vscode.SourceControlResourceState | vscode.SourceControlResourceGroup | vscode.Uri | any): Promise<void> {
    if (!resource || resource.id === 'staged' || Array.isArray(resource.resourceStates)) {
      await this.unstageAll();
      return;
    }
    const targetPath = resource.resourceUri?.fsPath || resource.fsPath;
    const relPath = targetPath ? path.relative(this.client.getWorkspaceRoot(), targetPath) : '';

    vscode.window.showInformationMessage(`Unstaged AST Component: ${relPath ? path.basename(relPath) : 'all'}`);
    await this.refresh();
  }

  /**
   * Unstages all staged components.
   */
  public async unstageAll(): Promise<void> {
    this.stagedGroup.resourceStates = [];
    vscode.window.showInformationMessage('Unstaged all AST components');
    await this.refresh();
  }

  /**
   * Commits staged AST changes (or auto-stages modified files if nothing is staged).
   */
  public async commit(): Promise<void> {
    let intent = this.scm.inputBox.value.trim();
    if (!intent) {
      const intentInput = await vscode.window.showInputBox({
        title: 'Cosm AST Commit: Intent',
        prompt: 'Enter commit message / intent',
        placeHolder: 'e.g. feat(auth): add token caching'
      });
      if (!intentInput || !intentInput.trim()) {
        return;
      }
      intent = intentInput.trim();
    }

    if (this.stagedGroup.resourceStates.length === 0) {
      if (this.modifiedGroup.resourceStates.length > 0) {
        const staged = await this.stageAll();
        if (!staged || this.stagedGroup.resourceStates.length === 0) {
          vscode.window.showWarningMessage('Staging failed or no changes to commit.');
          return;
        }
      } else {
        vscode.window.showInformationMessage('No changes to commit, working tree clean.');
        return;
      }
    }

    try {
      const result = await this.client.commit({
        universe: this.activeUniverse,
        intent,
        prompt: '',
        agent: 'cosm-human',
        model: 'human',
        sessionId: 'sess-' + Date.now().toString(16)
      });

      this.scm.inputBox.value = '';
      vscode.window.showInformationMessage(
        `🌟 Committed to '${this.activeUniverse}' (Merkle: ${result.merkle_root.substring(0, 12)}...)`
      );
      await this.refresh();
    } catch (err: any) {
      vscode.window.showErrorMessage(`Cosm commit failed: ${err.message}`);
    }
  }

  /**
   * Commits staged AST symbols with causal lineage (Intent + Originating Prompt).
   */
  public async commitWithPrompt(): Promise<void> {
    let intent = this.scm.inputBox.value.trim();
    if (!intent) {
      const intentInput = await vscode.window.showInputBox({
        title: 'Cosm AST Commit: Intent',
        prompt: 'Enter the commit intent description',
        placeHolder: 'e.g. feat(auth): implement JWT refresh token blacklist'
      });
      if (!intentInput) return;
      intent = intentInput.trim();
    }

    const userPrompt = await vscode.window.showInputBox({
      title: 'Cosm AST Lineage: Originating User Prompt',
      prompt: 'Enter the user prompt that originated this AST modification (stored in Merkle lineage)',
      placeHolder: 'e.g. "Add redis caching layer for JWT tokens with 15min expiry"'
    });
    if (!userPrompt) return;

    const agentId = await vscode.window.showInputBox({
      title: 'Cosm AST Lineage: Executing Agent ID (Optional)',
      prompt: 'Agent or developer identifier for causal pedigree',
      value: 'gemini-3.8-flash'
    });

    try {
      vscode.window.withProgress(
        {
          location: vscode.ProgressLocation.Notification,
          title: `Committing AST symbols to ${this.activeUniverse}...`,
          cancellable: false
        },
        async () => {
          const result = await this.client.commit({
            universe: this.activeUniverse,
            intent,
            prompt: userPrompt,
            agent: agentId || 'gemini-3.8-flash',
            model: 'gemini-3.8-flash',
            sessionId: `sess-${Date.now().toString(16)}`
          });

          this.scm.inputBox.value = '';
          vscode.window.showInformationMessage(
            `🌟 Committed to '${this.activeUniverse}' (Merkle: ${result.merkle_root.substring(0, 12)}...)`
          );
          await this.refresh();
        }
      );
    } catch (err: any) {
      vscode.window.showErrorMessage(`Cosm commit failed: ${err.message}`);
    }
  }

  /**
   * Opens side-by-side comparison between the universe head version and the working tree disk file.
   */
  public async diffSymbol(fileUri: vscode.Uri, relPath?: string): Promise<void> {
    const targetPath = relPath || path.relative(this.client.getWorkspaceRoot(), fileUri.fsPath);
    const leftUri = vscode.Uri.parse(
      `${CosmContentProvider.scheme}://${this.activeUniverse}/${targetPath}?symbol=${encodeURIComponent(targetPath)}`
    );
    const title = `${path.basename(targetPath)} (Cosm Head ↔ Working Tree)`;

    await vscode.commands.executeCommand('vscode.diff', leftUri, fileUri, title);
  }

  public dispose(): void {
    for (const d of this.disposables) {
      d.dispose();
    }
  }
}
