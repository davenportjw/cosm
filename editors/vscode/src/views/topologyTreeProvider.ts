import * as vscode from 'vscode';
import * as path from 'path';
import { CosmClient } from '../client/cosmClient';
import { FullTopologyGraph, TopologyNode, TopologyEdge } from '../types';

export type TopologyTreeElement = TierCategoryItem | TopologyComponentItem | TopologyNodeItem | TopologyEdgeItem | TopologyActionItem;

/**
 * Top-level tier category (Frontend, Backend, Cloud Infra).
 */
export class TierCategoryItem extends vscode.TreeItem {
  public readonly tierKey: 'Frontend' | 'API/Backend' | 'Cloud Infra';
  public readonly nodes: TopologyNode[];

  constructor(
    label: string,
    tierKey: 'Frontend' | 'API/Backend' | 'Cloud Infra',
    nodes: TopologyNode[],
    icon: string
  ) {
    super(label, vscode.TreeItemCollapsibleState.Expanded);
    this.tierKey = tierKey;
    this.nodes = nodes;
    const fileSet = new Set(nodes.map(n => n.file_path || n.component_name || 'default'));
    const fileCount = fileSet.size;
    if (nodes.length === 0) {
      this.description = '(0 components)';
    } else {
      this.description = `(${fileCount} ${fileCount === 1 ? 'file' : 'files'}, ${nodes.length} ${nodes.length === 1 ? 'symbol' : 'symbols'})`;
    }
    this.iconPath = new vscode.ThemeIcon(icon);
    this.contextValue = 'cosm.topologyTier';
    this.tooltip = `${label}\nContains ${fileCount} files and ${nodes.length} architectural symbols and components.`;
  }
}

/**
 * TreeItem representing a component or source file grouping symbols within a tier.
 */
export class TopologyComponentItem extends vscode.TreeItem {
  public readonly componentKey: string;
  public readonly filePath: string;
  public readonly nodes: TopologyNode[];
  public readonly workspaceRoot?: string;

  constructor(
    componentKey: string,
    filePath: string,
    nodes: TopologyNode[],
    workspaceRoot?: string
  ) {
    const symbolCount = nodes.length;
    const label = filePath || componentKey;
    super(label, vscode.TreeItemCollapsibleState.Expanded);

    this.componentKey = componentKey;
    this.filePath = filePath;
    this.nodes = nodes;
    this.workspaceRoot = workspaceRoot;

    this.description = `(${symbolCount} ${symbolCount === 1 ? 'symbol' : 'symbols'})`;
    this.tooltip = new vscode.MarkdownString(
      `### Component: ${componentKey}\n\n` +
      `- **File**: \`${filePath || 'inferred'}\`\n` +
      `- **Symbols**: ${symbolCount}\n\n` +
      `*Click to open source file, or expand to view symbols.*`
    );

    const lower = (filePath || '').toLowerCase();
    if (lower.endsWith('.tf') || lower.endsWith('.hcl')) {
      this.iconPath = new vscode.ThemeIcon('symbol-property');
    } else if (lower.includes('dockerfile')) {
      this.iconPath = new vscode.ThemeIcon('package');
    } else if (lower.endsWith('.sql')) {
      this.iconPath = new vscode.ThemeIcon('database');
    } else if (lower.endsWith('.go')) {
      this.iconPath = new vscode.ThemeIcon('symbol-namespace');
    } else if (lower.endsWith('.ts') || lower.endsWith('.tsx') || lower.endsWith('.js') || lower.endsWith('.jsx')) {
      this.iconPath = new vscode.ThemeIcon('browser');
    } else {
      this.iconPath = new vscode.ThemeIcon('file-code');
    }

    this.contextValue = 'cosm.topologyComponent';

    if (filePath) {
      this.command = {
        command: 'cosm.navigateToSymbol',
        title: 'Open File',
        arguments: [filePath]
      };
    }
  }
}

/**
 * Native TreeItem representing an architectural symbol / resource in a tier.
 */
export class TopologyNodeItem extends vscode.TreeItem {
  public readonly node: TopologyNode;
  public readonly workspaceRoot?: string;

  constructor(node: TopologyNode, workspaceRoot?: string) {
    const KNOWN_EXTENSIONS = new Set(['.tsx', '.ts', '.jsx', '.js', '.go', '.py', '.tf', '.hcl', '.sql', '.proto', '.json', '.yaml', '.yml', '.md', '.html', '.css']);
    let displayName = node.name;
    if (displayName.includes(':')) {
      displayName = displayName.split(':').pop()!;
    } else if (displayName.startsWith('resource.')) {
      displayName = displayName.substring('resource.'.length);
    } else if (displayName.startsWith('variable.') || displayName.startsWith('output.') || displayName.startsWith('provider.')) {
      displayName = displayName;
    } else {
      const ext = path.extname(displayName).toLowerCase();
      if (!KNOWN_EXTENSIONS.has(ext) && displayName.includes('.')) {
        displayName = displayName.split('.').pop()!;
      }
    }
    const hasChildren = (node.outgoing && node.outgoing.length > 0) || Boolean(node.file_path);
    super(displayName, hasChildren ? vscode.TreeItemCollapsibleState.Collapsed : vscode.TreeItemCollapsibleState.None);

    this.node = node;
    this.workspaceRoot = workspaceRoot;

    const edgeCount = node.outgoing ? node.outgoing.length : 0;
    const edgeText = edgeCount > 0 ? ` [${edgeCount} ${edgeCount === 1 ? 'contract' : 'contracts'}]` : '';
    this.description = `[${node.language}] ${node.type}${edgeText}`;

    this.tooltip = new vscode.MarkdownString(
      `### ${node.name}\n\n` +
      `- **Tier**: ${node.tier}\n` +
      `- **Type**: ${node.type}\n` +
      `- **Language**: ${node.language}\n` +
      `- **File**: \`${node.file_path || 'inferred'}\`\n` +
      `- **Contracts/Edges**: ${edgeCount}\n` +
      `- **Node ID**: \`${node.id}\`\n\n` +
      `*Click to jump to declaration in source file.*`
    );

    this.iconPath = getTierNodeIcon(node);
    this.contextValue = 'cosm.topologyNode';

    // Click command to navigate directly to symbol
    this.command = {
      command: 'cosm.navigateToSymbol',
      title: 'Navigate to Symbol',
      arguments: [node.file_path || node.name, node.name]
    };
  }
}

/**
 * TreeItem representing a cross-domain contract / edge (e.g. CONSUMES_API, BINDS_ENV).
 */
export class TopologyEdgeItem extends vscode.TreeItem {
  public readonly edge: TopologyEdge;

  constructor(edge: TopologyEdge) {
    super(`🔗 ${edge.edge_type}`, vscode.TreeItemCollapsibleState.None);
    this.edge = edge;
    this.description = edge.label ? `${edge.label} → ${edge.target_id}` : `→ ${edge.target_id}`;
    this.tooltip = `Contract Type: ${edge.edge_type}\nLabel: ${edge.label || 'None'}\nTarget: ${edge.target_id}`;
    this.iconPath = new vscode.ThemeIcon('references');
    this.contextValue = 'cosm.topologyEdge';
  }
}

/**
 * Action item for auditing blast radius directly from the tree view.
 */
export class TopologyActionItem extends vscode.TreeItem {
  constructor(label: string, icon: string, command: string, args: any[], tooltip: string) {
    super(label, vscode.TreeItemCollapsibleState.None);
    this.iconPath = new vscode.ThemeIcon(icon);
    this.command = {
      command,
      title: label,
      arguments: args
    };
    this.tooltip = tooltip;
    this.contextValue = 'cosm.topologyAction';
  }
}

function getTierNodeIcon(node: TopologyNode): vscode.ThemeIcon {
  switch (node.tier) {
    case 'Frontend':
      return new vscode.ThemeIcon('browser');
    case 'Cloud Infra':
      return new vscode.ThemeIcon('cloud');
    case 'API/Backend':
    case 'Backend':
    default:
      if (node.type.toLowerCase().includes('struct') || node.type.toLowerCase().includes('class')) {
        return new vscode.ThemeIcon('symbol-structure');
      }
      return new vscode.ThemeIcon('symbol-method');
  }
}

/**
 * Native VS Code TreeDataProvider for 3-Tier Architecture Topology.
 * Completely replaces the iframe webview in the sidebar with native VS Code widgets.
 */
export class CosmTopologyTreeDataProvider implements vscode.TreeDataProvider<TopologyTreeElement> {
  private _onDidChangeTreeData: vscode.EventEmitter<TopologyTreeElement | undefined | null | void> =
    new vscode.EventEmitter<TopologyTreeElement | undefined | null | void>();
  readonly onDidChangeTreeData: vscode.Event<TopologyTreeElement | undefined | null | void> =
    this._onDidChangeTreeData.event;

  private client: CosmClient;
  private cachedGraph: FullTopologyGraph | null = null;

  constructor(client: CosmClient) {
    this.client = client;
  }

  public refresh(): void {
    this.cachedGraph = null;
    this._onDidChangeTreeData.fire();
  }

  public getTreeItem(element: TopologyTreeElement): vscode.TreeItem {
    return element;
  }

  public async getChildren(element?: TopologyTreeElement): Promise<TopologyTreeElement[]> {
    if (!element) {
      try {
        const graph = await this.client.getTopology();
        this.cachedGraph = graph;

        const frontendNodes = graph.frontend_nodes || [];
        const backendNodes = graph.backend_nodes || [];
        const infraNodes = graph.infra_nodes || [];

        return [
          new TierCategoryItem('🌐 Frontend Tier (Web / UI)', 'Frontend', frontendNodes, 'browser'),
          new TierCategoryItem('⚙️ Backend & API Tier (Services / Store)', 'API/Backend', backendNodes, 'server'),
          new TierCategoryItem('☁️ Cloud Infrastructure Tier (IaC)', 'Cloud Infra', infraNodes, 'cloud')
        ];
      } catch (err: any) {
        vscode.window.showErrorMessage(`Failed to load Architecture Topology: ${err.message}`);
        return [];
      }
    }

    if (element instanceof TierCategoryItem) {
      const workspaceRoot = this.client.getWorkspaceRoot();
      // Group nodes by file_path or component_name
      const groups = new Map<string, TopologyNode[]>();
      for (const node of element.nodes) {
        const key = node.file_path || node.component_name || 'component';
        if (!groups.has(key)) {
          groups.set(key, []);
        }
        groups.get(key)!.push(node);
      }

      const items: TopologyTreeElement[] = [];
      for (const [key, nodes] of groups.entries()) {
        const filePath = nodes[0]?.file_path || key;
        const compName = nodes[0]?.component_name || key;
        items.push(new TopologyComponentItem(compName, filePath, nodes, workspaceRoot));
      }
      return items;
    }

    if (element instanceof TopologyComponentItem) {
      const workspaceRoot = element.workspaceRoot;
      return element.nodes.map(n => new TopologyNodeItem(n, workspaceRoot));
    }

    if (element instanceof TopologyNodeItem) {
      const children: TopologyTreeElement[] = [];

      // 1. Audit Blast Radius Action
      children.push(
        new TopologyActionItem(
          'Audit Blast Radius',
          'pulse',
          'cosm.blastRadius',
          [element.node.id],
          `Calculate downstream blast radius for ${element.node.name}`
        )
      );

      // 2. Open File Action
      if (element.node.file_path) {
        const absPath = path.isAbsolute(element.node.file_path)
          ? element.node.file_path
          : (element.workspaceRoot ? path.resolve(element.workspaceRoot, element.node.file_path) : element.node.file_path);

        children.push(
          new TopologyActionItem(
            `File: ${element.node.file_path}`,
            'go-to-file',
            'vscode.open',
            [vscode.Uri.file(absPath)],
            `Open source file ${element.node.file_path}`
          )
        );
      }

      // 3. Outgoing Contract Edges
      if (element.node.outgoing && element.node.outgoing.length > 0) {
        for (const edge of element.node.outgoing) {
          children.push(new TopologyEdgeItem(edge));
        }
      }

      return children;
    }

    return [];
  }
}
