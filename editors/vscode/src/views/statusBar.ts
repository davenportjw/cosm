import * as vscode from 'vscode';
import { CosmClient } from '../client/cosmClient';
import { CosmSCMProvider } from '../scm/provider';

export class CosmStatusBar implements vscode.Disposable {
  private statusBarItem: vscode.StatusBarItem;
  private client: CosmClient;
  private scmProvider: CosmSCMProvider;
  private activeUniverse: string = 'universe-main';

  constructor(client: CosmClient, scmProvider: CosmSCMProvider) {
    this.client = client;
    this.scmProvider = scmProvider;

    this.statusBarItem = vscode.window.createStatusBarItem(
      vscode.StatusBarAlignment.Left,
      100
    );
    this.statusBarItem.command = 'cosm.switchUniverse';
    this.updateText();
    this.statusBarItem.show();
  }

  public setActiveUniverse(universeId: string): void {
    this.activeUniverse = universeId;
    this.updateText();
  }

  public getActiveUniverse(): string {
    return this.activeUniverse;
  }

  /**
   * Refreshes status bar metrics and tooltip from Cosm CLI.
   */
  public async update(): Promise<void> {
    try {
      const status = await this.client.getStatus(this.activeUniverse);
      this.activeUniverse = status.universe_id;
      this.updateText();

      const tooltip = new vscode.MarkdownString();
      tooltip.isTrusted = true;
      tooltip.appendMarkdown(`### 🌌 Cosm AST Micro-Universe: **${this.activeUniverse}**\n\n`);
      tooltip.appendMarkdown(`- **Head Merkle Root**: \`${status.merkle_root ? status.merkle_root.substring(0, 16) + '...' : 'none'}\`\n`);
      tooltip.appendMarkdown(`- **Tracked AST Components**: **${status.components_count}**\n`);
      tooltip.appendMarkdown(`- **Cross-Boundary Edges**: **${status.cross_edges_count}**\n`);
      tooltip.appendMarkdown(`- **Topocosm Hub**: Connected (\`topocosm.dev\`)\n\n`);
      tooltip.appendMarkdown(`---\n*Click to switch micro-universe, create branch, or open topology.*`);
      this.statusBarItem.tooltip = tooltip;
    } catch (err: any) {
      this.statusBarItem.text = `$(alert) Cosm [${this.activeUniverse}]`;
      this.statusBarItem.tooltip = `Cosm CLI error: ${err.message}`;
    }
  }

  private updateText(): void {
    this.statusBarItem.text = `$(globe) [🌌 ${this.activeUniverse}]`;
  }

  /**
   * Shows interactive QuickPick modal allowing switching, branching, and Jujutsu-stack inspection.
   */
  public async showUniversePicker(): Promise<void> {
    interface QuickPickActionItem extends vscode.QuickPickItem {
      action: 'switch' | 'create' | 'merge' | 'log' | 'stack' | 'topology' | 'ship' | 'blast' | 'refresh';
      universeId?: string;
    }

    const items: QuickPickActionItem[] = [];

    try {
      const universes = await this.client.listUniverses();
      items.push({
        label: 'Active Micro-Universes',
        kind: vscode.QuickPickItemKind.Separator,
        action: 'switch'
      });

      for (const u of universes) {
        const isCurrent = u.universe_id === this.activeUniverse;
        items.push({
          label: `${isCurrent ? '• ' : '  '}${u.universe_id}`,
          description: `Head: ${u.head_manifest_hash.substring(0, 8)}... (${u.status})`,
          detail: isCurrent ? '✓ Currently active micro-universe' : undefined,
          action: 'switch',
          universeId: u.universe_id
        });
      }
    } catch {
      // fallback if universe listing fails
    }

    items.push({
      label: 'Actions & Tools',
      kind: vscode.QuickPickItemKind.Separator,
      action: 'create'
    });

    items.push({
      label: '$(add) Create New Micro-Universe...',
      description: 'Zero-copy branch from current head',
      action: 'create'
    });

    items.push({
      label: '$(git-merge) Merge Micro-Universe into Target...',
      description: 'AST Component & Cross-Edge Union Merge or Fast-Forward',
      action: 'merge'
    });

    items.push({
      label: '$(history) View Commit History & Log...',
      description: 'View micro-universe commit lineage and pedigree',
      action: 'log'
    });

    items.push({
      label: '$(layers) Inspect Stacked Proposals (Jujutsu-style)...',
      description: 'View chained micro-universe changes',
      action: 'stack'
    });

    items.push({
      label: '$(type-hierarchy-sub) Open Cross-Domain Architecture Canvas...',
      description: '3-tier visual map: Frontend → Backend → Cloud Infra',
      action: 'topology'
    });

    items.push({
      label: '$(rocket) Ship Ephemeral Target Preview...',
      description: 'Run cosm ship sidecar build',
      action: 'ship'
    });

    items.push({
      label: '$(pulse) Audit Blast Radius...',
      description: 'Inspect downstream symbol contract dependencies',
      action: 'blast'
    });

    items.push({
      label: '$(refresh) Refresh Cosm AST State',
      description: 'Reload status and Merkle heads from store',
      action: 'refresh'
    });

    const selected = await vscode.window.showQuickPick(items, {
      placeHolder: `Select action or switch micro-universe (Current: ${this.activeUniverse})`,
      matchOnDescription: true,
      matchOnDetail: true
    });

    if (!selected) return;

    switch (selected.action) {
      case 'switch':
        if (selected.universeId && selected.universeId !== this.activeUniverse) {
          this.setActiveUniverse(selected.universeId);
          this.scmProvider.setActiveUniverse(selected.universeId);
          try {
            await this.client.switchUniverse(selected.universeId);
          } catch {}
          await this.update();
          vscode.window.showInformationMessage(`Switched to micro-universe: ${selected.universeId}`);
        }
        break;

      case 'merge':
        await vscode.commands.executeCommand('cosm.mergeUniverse');
        break;

      case 'log':
        await vscode.commands.executeCommand('cosm.showLog');
        break;

      case 'create':
        await this.promptCreateUniverse();
        break;

      case 'stack':
        await this.showStackedProposals();
        break;

      case 'topology':
        await vscode.commands.executeCommand('cosm.openTopology');
        break;

      case 'ship':
        await vscode.commands.executeCommand('cosm.shipPreview');
        break;

      case 'blast':
        await vscode.commands.executeCommand('cosm.blastRadius');
        break;

      case 'refresh':
        await this.scmProvider.refresh();
        await this.update();
        vscode.window.showInformationMessage('Cosm AST state refreshed.');
        break;
    }
  }

  /**
   * Prompts user for new universe ID and creates it zero-copy.
   */
  public async promptCreateUniverse(): Promise<void> {
    const newId = await vscode.window.showInputBox({
      title: 'Cosm: Create Micro-Universe',
      prompt: 'Enter unique universe identifier (e.g. u/feature-payment-gateway)',
      placeHolder: 'u/feature-branch-1'
    });

    if (!newId) return;

    try {
      const res = await this.client.createUniverse(newId.trim(), this.activeUniverse);
      vscode.window.showInformationMessage(
        `✨ Created micro-universe '${newId}' branched from '${this.activeUniverse}'`
      );
      this.setActiveUniverse(newId.trim());
      this.scmProvider.setActiveUniverse(newId.trim());
      await this.update();
    } catch (err: any) {
      vscode.window.showErrorMessage(`Failed to create micro-universe: ${err.message}`);
    }
  }

  /**
   * Displays Jujutsu-style stacked proposals in QuickPick.
   */
  public async showStackedProposals(): Promise<void> {
    try {
      const stacks = await this.client.listStack();
      if (stacks.length === 0) {
        vscode.window.showInformationMessage(
          `No stacked proposals active on '${this.activeUniverse}'. Create stacks via 'cosm stack create'.`
        );
        return;
      }

      const items = stacks.map(s => ({
        label: `$(git-commit) ${s.change_id}`,
        description: `Universe: ${s.universe_id}`,
        detail: s.title || 'Stacked change'
      }));

      await vscode.window.showQuickPick(items, {
        title: 'Cosm: Jujutsu-Style Stacked Proposals',
        placeHolder: 'Chained dependent changes'
      });
    } catch (err: any) {
      vscode.window.showErrorMessage(`Failed to query stacks: ${err.message}`);
    }
  }

  public dispose(): void {
    this.statusBarItem.dispose();
  }
}
