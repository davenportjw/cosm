import * as vscode from 'vscode';
import * as path from 'path';
import { CosmClient } from '../client/cosmClient';
import {
  ASTTreeGraph,
  ASTTreeComponent,
  ASTTreeSymbol,
  ASTTreeEdge,
  ASTTreeLineage
} from '../types';

export type ASTTreeItem =
  | UniverseItem
  | DirectoryItem
  | ComponentItem
  | SymbolItem
  | DetailItem;

/**
 * Returns the appropriate VS Code ThemeIcon based on the AST symbol's node type.
 */
export function getSymbolThemeIcon(nodeType: string): vscode.ThemeIcon {
  const t = (nodeType || '').toLowerCase();
  if (t.includes('func') || t.includes('method')) {
    return new vscode.ThemeIcon('symbol-method');
  }
  if (
    t.includes('struct') ||
    t.includes('class') ||
    t.includes('interface') ||
    t.includes('type') ||
    t.includes('componentdecl')
  ) {
    return new vscode.ThemeIcon('symbol-structure');
  }
  if (
    t.includes('resource') ||
    t.includes('infra') ||
    t.includes('cloud') ||
    t.includes('bucket') ||
    t.includes('database') ||
    t.includes('table')
  ) {
    return new vscode.ThemeIcon('cloud');
  }
  if (
    t.includes('endpoint') ||
    t.includes('route') ||
    t.includes('api') ||
    t.includes('http') ||
    t.includes('web') ||
    t.includes('globe')
  ) {
    return new vscode.ThemeIcon('globe');
  }
  if (
    t.includes('var') ||
    t.includes('field') ||
    t.includes('const') ||
    t.includes('prop')
  ) {
    return new vscode.ThemeIcon('symbol-variable');
  }
  return new vscode.ThemeIcon('symbol-variable');
}

/**
 * Root TreeItem representing a Micro-Universe and its Merkle root.
 */
export class UniverseItem extends vscode.TreeItem {
  public readonly graph: ASTTreeGraph;

  constructor(graph: ASTTreeGraph) {
    const shortMerkle = graph.merkle_root
      ? graph.merkle_root.length > 8
        ? graph.merkle_root.substring(0, 8) + '...'
        : graph.merkle_root
      : 'none';
    super(
      `🌌 ${graph.universe_id} [Merkle: ${shortMerkle}]`,
      vscode.TreeItemCollapsibleState.Expanded
    );
    this.graph = graph;
    this.description = `(${graph.total_components} components, ${graph.total_symbols} symbols, ${graph.total_edges} edges)`;
    this.tooltip = `Universe: ${graph.universe_id}\nMerkle Root: ${graph.merkle_root}\nDeterministic SHA-256 cryptographic root of the AST Merkle-DAG.\nGuarantees zero-tamper integrity across all polyglot symbols and contracts.\nTotal Components: ${graph.total_components}\nTotal Symbols: ${graph.total_symbols}\nCross Edges: ${graph.total_edges}`;
    this.iconPath = new vscode.ThemeIcon('globe');
    this.contextValue = 'cosm.universe';
  }
}

/**
 * Educational TreeItem communicating what AST Merkle Nodes and Merkle roots are.
 */
export class MerkleInfoItem extends vscode.TreeItem {
  constructor(merkleRoot?: string) {
    super('ℹ️ What is an AST Merkle Node?', vscode.TreeItemCollapsibleState.None);
    const shortRoot = merkleRoot && merkleRoot !== 'none'
      ? (merkleRoot.length > 12 ? merkleRoot.substring(0, 12) + '...' : merkleRoot)
      : 'immutable';
    this.description = `[Root: ${shortRoot}] Click to learn`;
    this.tooltip = new vscode.MarkdownString(
      `### Cosm AST Merkle-DAG Architecture\n\n` +
      `In Cosm, code is **not** stored as line diffs or plain text files. Instead, software is stored as a **content-addressed AST Merkle-DAG**:\n\n` +
      `- **AST Symbol Merkle Nodes**: Every function, struct, and resource is hashed via SHA-256 over its normalized syntax tree.\n` +
      `- **Hierarchical Folding**: Symbol hashes roll up deterministically into Component Merkle Nodes, which roll up into the **Universe Merkle Root**.\n` +
      `- **Cryptographic Lineage**: Ed25519 digital signatures bind AI prompts, agents, and sessions directly to AST nodes.\n` +
      `- **Zero-Copy Micro-Universes**: Unchanged subtrees share Merkle hashes across parallel universes.\n\n` +
      `*Click to open the interactive Merkle DAG documentation & visualizer.*`
    );
    this.iconPath = new vscode.ThemeIcon('shield');
    this.contextValue = 'cosm.merkleInfo';
    this.command = {
      command: 'cosm.explainMerkleNodes',
      title: 'Explain AST Merkle Nodes'
    };
  }
}

/**
 * Directory TreeItem for collapsible folder hierarchies (e.g. pkg/core).
 */
export class DirectoryItem extends vscode.TreeItem {
  public readonly dirPath: string;
  public subDirectories: Map<string, DirectoryItem> = new Map();
  public components: ComponentItem[] = [];

  constructor(dirPath: string, label?: string) {
    super(label || dirPath, vscode.TreeItemCollapsibleState.Collapsed);
    this.dirPath = dirPath;
    this.iconPath = vscode.ThemeIcon.Folder;
    this.contextValue = 'cosm.directory';
    this.tooltip = `Directory: ${dirPath}`;
  }
}

/**
 * Component TreeItem representing source files with language badges and symbol counts.
 */
export class ComponentItem extends vscode.TreeItem {
  public readonly component: ASTTreeComponent;
  public readonly filePath: string;

  constructor(component: ASTTreeComponent, filePath: string, workspaceRoot?: string) {
    const fileName = path.basename(filePath || component.name || component.component_id);
    super(
      fileName,
      component.symbols && component.symbols.length > 0
        ? vscode.TreeItemCollapsibleState.Collapsed
        : vscode.TreeItemCollapsibleState.None
    );
    this.component = component;
    this.filePath = filePath;

    const symCount = component.symbols ? component.symbols.length : 0;
    const lang = component.language || 'polyglot';
    this.description = `[${lang}] (${symCount} ${symCount === 1 ? 'symbol' : 'symbols'})`;
    this.tooltip = `Component Merkle Node: ${filePath}\nLanguage: ${lang}\nType: ${component.type || 'Component'}\nSymbols: ${symCount}\nAggregates child AST symbol hashes into component Merkle tree.`;
    this.iconPath = new vscode.ThemeIcon('file-code');
    this.contextValue = 'cosm.component';

    const absPath = path.isAbsolute(filePath)
      ? filePath
      : (workspaceRoot ? path.resolve(workspaceRoot, filePath) : filePath);
    this.resourceUri = vscode.Uri.file(absPath);
    this.command = {
      command: 'vscode.open',
      title: 'Open File',
      arguments: [this.resourceUri]
    };
  }
}

/**
 * Symbol TreeItem representing functions, structs, cloud resources, endpoints, etc.
 */
export class SymbolItem extends vscode.TreeItem {
  public readonly symbol: ASTTreeSymbol;
  public readonly filePath: string;

  constructor(symbol: ASTTreeSymbol, filePath: string) {
    super(symbol.identifier, vscode.TreeItemCollapsibleState.Collapsed);
    this.symbol = symbol;
    this.filePath = filePath;
    this.description = symbol.signature ? symbol.signature : symbol.node_type;
    this.tooltip = `Symbol: ${symbol.identifier}\nType: ${symbol.node_type}\nLanguage: ${symbol.language}\nNode ID: ${symbol.node_id}${symbol.signature ? `\nSignature: ${symbol.signature}` : ''}`;
    this.iconPath = getSymbolThemeIcon(symbol.node_type);
    this.contextValue = 'cosm.symbol';
    this.command = {
      command: 'cosm.navigateToSymbol',
      title: 'Navigate to Symbol',
      arguments: [this.filePath, this.symbol]
    };
  }
}

/**
 * Detail TreeItem representing expandable signature, contracts/edges, lineage, and node ID.
 */
export class DetailItem extends vscode.TreeItem {
  public children?: DetailItem[];
  public value?: string;

  constructor(
    label: string,
    description?: string,
    icon?: vscode.ThemeIcon,
    collapsibleState: vscode.TreeItemCollapsibleState = vscode.TreeItemCollapsibleState.None,
    children?: DetailItem[],
    contextValue?: string,
    value?: string
  ) {
    super(label, collapsibleState);
    this.description = description;
    if (icon) {
      this.iconPath = icon;
    }
    this.children = children;
    if (contextValue) {
      this.contextValue = contextValue;
    }
    this.value = value;
  }
}

/**
 * Helper to construct the detailed sub-items for a given AST Symbol.
 */
export function createSymbolDetails(symbol: ASTTreeSymbol): DetailItem[] {
  const details: DetailItem[] = [];

  // 1. Signature
  if (symbol.signature) {
    const sigChildren: DetailItem[] = [];
    if (symbol.docstring) {
      sigChildren.push(
        new DetailItem(
          'Docstring',
          symbol.docstring,
          new vscode.ThemeIcon('note'),
          vscode.TreeItemCollapsibleState.None
        )
      );
    }
    details.push(
      new DetailItem(
        'Signature',
        symbol.signature,
        new vscode.ThemeIcon('symbol-key'),
        sigChildren.length > 0
          ? vscode.TreeItemCollapsibleState.Collapsed
          : vscode.TreeItemCollapsibleState.None,
        sigChildren,
        'cosm.detailSignature',
        symbol.signature
      )
    );
  } else {
    details.push(
      new DetailItem(
        'Signature',
        `${symbol.node_type} ${symbol.identifier}`,
        new vscode.ThemeIcon('symbol-key'),
        vscode.TreeItemCollapsibleState.None,
        undefined,
        'cosm.detailSignature',
        `${symbol.node_type} ${symbol.identifier}`
      )
    );
  }

  // 2. Contracts / Edges with 'references' icon
  if (symbol.outgoing_edges && symbol.outgoing_edges.length > 0) {
    const edgeChildren = symbol.outgoing_edges.map(edge => {
      const edgeLabel = `${edge.edge_type}`;
      const edgeDesc = edge.label ? `${edge.label} → ${edge.target_id}` : `→ ${edge.target_id}`;
      return new DetailItem(
        edgeLabel,
        edgeDesc,
        new vscode.ThemeIcon('references'),
        vscode.TreeItemCollapsibleState.None,
        undefined,
        'cosm.detailEdge',
        edge.target_id
      );
    });
    details.push(
      new DetailItem(
        'Contracts/Edges',
        `(${symbol.outgoing_edges.length})`,
        new vscode.ThemeIcon('references'),
        vscode.TreeItemCollapsibleState.Collapsed,
        edgeChildren,
        'cosm.detailContracts'
      )
    );
  } else {
    details.push(
      new DetailItem(
        'Contracts/Edges',
        'None',
        new vscode.ThemeIcon('references'),
        vscode.TreeItemCollapsibleState.None,
        undefined,
        'cosm.detailContracts'
      )
    );
  }

  // 3. Lineage with 'history' icon
  if (symbol.lineage) {
    const lineageChildren: DetailItem[] = [];
    if (symbol.lineage.user_prompt) {
      lineageChildren.push(
        new DetailItem(
          'Prompt',
          `"${symbol.lineage.user_prompt}"`,
          new vscode.ThemeIcon('comment'),
          vscode.TreeItemCollapsibleState.None
        )
      );
    }
    if (symbol.lineage.intent) {
      lineageChildren.push(
        new DetailItem(
          'Intent',
          symbol.lineage.intent,
          new vscode.ThemeIcon('git-commit'),
          vscode.TreeItemCollapsibleState.None
        )
      );
    }
    if (symbol.lineage.executing_agent_id) {
      lineageChildren.push(
        new DetailItem(
          'Agent',
          symbol.lineage.executing_agent_id,
          new vscode.ThemeIcon('hubot'),
          vscode.TreeItemCollapsibleState.None
        )
      );
    }
    if (symbol.lineage.timestamp) {
      lineageChildren.push(
        new DetailItem(
          'Timestamp',
          symbol.lineage.timestamp,
          new vscode.ThemeIcon('calendar'),
          vscode.TreeItemCollapsibleState.None
        )
      );
    }
    const agentBadge = symbol.lineage.executing_agent_id
      ? `[Agent: ${symbol.lineage.executing_agent_id}]`
      : '';
    details.push(
      new DetailItem(
        'Lineage',
        agentBadge,
        new vscode.ThemeIcon('history'),
        vscode.TreeItemCollapsibleState.Collapsed,
        lineageChildren,
        'cosm.detailLineage'
      )
    );
  } else {
    details.push(
      new DetailItem(
        'Lineage',
        'Unsigned / No Pedigree',
        new vscode.ThemeIcon('history'),
        vscode.TreeItemCollapsibleState.None,
        undefined,
        'cosm.detailLineage'
      )
    );
  }

  // 4. Node ID with 'key' icon
  const nodeIdItem = new DetailItem(
    'Node ID',
    symbol.node_id,
    new vscode.ThemeIcon('key'),
    vscode.TreeItemCollapsibleState.None,
    undefined,
    'cosm.detailNodeId',
    symbol.node_id
  );
  nodeIdItem.command = {
    command: 'cosm.copyNodeId',
    title: 'Copy Node ID',
    arguments: [symbol.node_id]
  };
  nodeIdItem.tooltip = `Click to copy node ID: ${symbol.node_id}`;
  details.push(nodeIdItem);

  return details;
}

interface DirNode {
  name: string;
  fullPath: string;
  subDirs: Map<string, DirNode>;
  components: ComponentItem[];
}

/**
 * TreeDataProvider for Cosm AST Merkle Tree Explorer.
 */
export class CosmASTTreeDataProvider implements vscode.TreeDataProvider<ASTTreeItem> {
  private _onDidChangeTreeData: vscode.EventEmitter<ASTTreeItem | undefined | null | void> =
    new vscode.EventEmitter<ASTTreeItem | undefined | null | void>();
  readonly onDidChangeTreeData: vscode.Event<ASTTreeItem | undefined | null | void> =
    this._onDidChangeTreeData.event;

  private client: CosmClient;
  private activeUniverse: string;
  private cachedGraph: ASTTreeGraph | null = null;

  constructor(client: CosmClient, initialUniverse: string = 'universe-main') {
    this.client = client;
    this.activeUniverse = initialUniverse;
  }

  public refresh(universe?: string): void {
    if (universe) {
      this.activeUniverse = universe;
    }
    this.cachedGraph = null;
    this._onDidChangeTreeData.fire();
  }

  public getTreeItem(element: ASTTreeItem): vscode.TreeItem {
    return element;
  }

  public async getChildren(element?: ASTTreeItem): Promise<ASTTreeItem[]> {
    if (!element) {
      try {
        const graph = await this.client.getASTTree(this.activeUniverse);
        this.cachedGraph = graph;
        return [new UniverseItem(graph)];
      } catch (err: any) {
        vscode.window.showErrorMessage(`Failed to load AST Tree: ${err.message}`);
        return [];
      }
    }

    if (element instanceof UniverseItem) {
      const graph = element.graph || this.cachedGraph;
      if (!graph || !graph.components || graph.components.length === 0) {
        return [];
      }
      const merkleInfo = new MerkleInfoItem(graph.merkle_root);
      return [merkleInfo, ...this.buildComponentHierarchy(graph.components)];
    }

    if (element instanceof DirectoryItem) {
      return [...element.subDirectories.values(), ...element.components];
    }

    if (element instanceof ComponentItem) {
      if (!element.component.symbols || element.component.symbols.length === 0) {
        return [];
      }
      return element.component.symbols.map(s => new SymbolItem(s, element.filePath));
    }

    if (element instanceof SymbolItem) {
      return createSymbolDetails(element.symbol);
    }

    if (element instanceof DetailItem) {
      return element.children || [];
    }

    return [];
  }

  /**
   * Builds directory and file hierarchy from a list of components,
   * compacting single-child folder chains (e.g. pkg/core).
   */
  public buildComponentHierarchy(components: ASTTreeComponent[]): ASTTreeItem[] {
    const rootNode: DirNode = {
      name: '',
      fullPath: '',
      subDirs: new Map(),
      components: []
    };

    const workspaceRoot = this.client ? this.client.getWorkspaceRoot() : undefined;

    for (const comp of components) {
      let compPath = (comp.component_id || comp.name || '').replace(/\\/g, '/');
      if (compPath.startsWith('./')) {
        compPath = compPath.substring(2);
      }
      if (compPath.startsWith('/')) {
        compPath = compPath.substring(1);
      }

      const parts = compPath.split('/').filter(Boolean);
      const compItem = new ComponentItem(comp, compPath, workspaceRoot);

      if (parts.length <= 1) {
        rootNode.components.push(compItem);
      } else {
        let curr = rootNode;
        let accumulated = '';
        for (let i = 0; i < parts.length - 1; i++) {
          const seg = parts[i];
          accumulated = accumulated ? `${accumulated}/${seg}` : seg;
          if (!curr.subDirs.has(seg)) {
            curr.subDirs.set(seg, {
              name: seg,
              fullPath: accumulated,
              subDirs: new Map(),
              components: []
            });
          }
          curr = curr.subDirs.get(seg)!;
        }
        curr.components.push(compItem);
      }
    }

    // Compact single-child directory chains without direct components
    this.compactTrie(rootNode);

    // Convert DirNodes into DirectoryItems
    const result: ASTTreeItem[] = [];
    for (const dirNode of rootNode.subDirs.values()) {
      result.push(this.dirNodeToItem(dirNode));
    }
    result.push(...rootNode.components);
    return result;
  }

  private compactTrie(node: DirNode): void {
    for (const [key, child] of Array.from(node.subDirs.entries())) {
      this.compactTrie(child);
      if (child.components.length === 0 && child.subDirs.size === 1) {
        const [grandchildKey, grandchild] = Array.from(child.subDirs.entries())[0];
        const mergedName = `${child.name}/${grandchild.name}`;
        grandchild.name = mergedName;
        node.subDirs.delete(key);
        node.subDirs.set(mergedName, grandchild);
      }
    }
  }

  private dirNodeToItem(node: DirNode): DirectoryItem {
    const item = new DirectoryItem(node.fullPath, node.name);
    for (const child of node.subDirs.values()) {
      item.subDirectories.set(child.fullPath, this.dirNodeToItem(child));
    }
    item.components = [...node.components];
    return item;
  }
}
