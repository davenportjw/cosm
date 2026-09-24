import * as vscode from 'vscode';
import { CosmClient } from '../client/cosmClient';
import { CosmSCMProvider } from '../scm/provider';
import { StackRecord } from '../types';

export class StackedChangeTreeItem extends vscode.TreeItem {
  public readonly record?: StackRecord;

  constructor(
    record?: StackRecord,
    index: number = 0,
    isActive: boolean = false
  ) {
    if (!record) {
      super('No active stacked changes', vscode.TreeItemCollapsibleState.None);
      this.description = 'Jujutsu-style stacked proposals';
      this.tooltip = 'Create a stacked change with cosm.createStack';
      this.iconPath = new vscode.ThemeIcon('info');
      this.contextValue = 'noStackedChanges';
      return;
    }

    const orderIdx = record.order_index ?? (index + 1);
    const activeSuffix = isActive ? ' (Active)' : '';
    const label = `🥞 [${orderIdx}] ${record.change_id}${activeSuffix}`;

    super(label, vscode.TreeItemCollapsibleState.None);
    this.record = record;

    if (isActive) {
      this.iconPath = new vscode.ThemeIcon('pass-filled');
    } else {
      this.iconPath = new vscode.ThemeIcon('layers');
    }

    const shortHash = (record.manifest_hash || '').substring(0, 8) || 'root';
    this.description = `${record.universe_id} | Head: ${shortHash}`;

    this.tooltip = [
      `Change ID: ${record.change_id}`,
      `Title: ${record.title || ''}`,
      `Universe: ${record.universe_id}`,
      `Parent: ${record.parent_change_id || 'universe-main'}`,
      `Status: ${record.status || 'Active'}`
    ].join('\n');

    this.contextValue = 'stackedChange';

    this.command = {
      command: 'cosm.switchUniverse',
      title: 'Switch Micro-Universe',
      arguments: [record.universe_id]
    };
  }
}

export class CosmStackedChangesProvider implements vscode.TreeDataProvider<StackedChangeTreeItem> {
  private _onDidChangeTreeData: vscode.EventEmitter<StackedChangeTreeItem | undefined | null | void> =
    new vscode.EventEmitter<StackedChangeTreeItem | undefined | null | void>();
  readonly onDidChangeTreeData: vscode.Event<StackedChangeTreeItem | undefined | null | void> =
    this._onDidChangeTreeData.event;

  private client: CosmClient;
  private scmProvider?: CosmSCMProvider;

  constructor(client: CosmClient, scmProvider?: CosmSCMProvider) {
    this.client = client;
    this.scmProvider = scmProvider;
  }

  public refresh(): void {
    this._onDidChangeTreeData.fire();
  }

  public getTreeItem(element: StackedChangeTreeItem): vscode.TreeItem {
    return element;
  }

  public async getChildren(element?: StackedChangeTreeItem): Promise<StackedChangeTreeItem[]> {
    if (element) {
      return [];
    }

    try {
      const records = await this.client.listStack();
      if (!records || records.length === 0) {
        return [new StackedChangeTreeItem()];
      }

      const activeUniverse = this.scmProvider ? this.scmProvider.getActiveUniverse() : 'universe-main';

      return records.map((record, index) => {
        const isActive = record.universe_id === activeUniverse;
        return new StackedChangeTreeItem(record, index, isActive);
      });
    } catch (err: any) {
      const errorItem = new StackedChangeTreeItem();
      errorItem.label = 'Error loading stacked proposals';
      errorItem.description = err.message;
      errorItem.tooltip = `Error: ${err.message}`;
      errorItem.iconPath = new vscode.ThemeIcon('error');
      return [errorItem];
    }
  }
}
