import * as cp from 'child_process';
import * as path from 'path';
import * as fs from 'fs';
import * as vscode from 'vscode';
import {
  CosmStatus,
  UniverseRecord,
  CommitLogEntry,
  ResolvedSymbol,
  FullTopologyGraph,
  TopologyNode,
  BlastRadiusReport,
  ShipResult,
  StackRecord,
  ASTTreeEdge,
  ASTTreeLineage,
  ASTTreeSymbol,
  ASTTreeComponent,
  ASTTreeGraph
} from '../types';

export interface CommitOptions {
  universe?: string;
  intent: string;
  prompt?: string;
  agent?: string;
  model?: string;
  sessionId?: string;
}

export class CosmClient {
  private workspaceRoot: string;

  constructor(workspaceRoot?: string) {
    if (workspaceRoot) {
      this.workspaceRoot = workspaceRoot;
    } else {
      const folders = vscode.workspace.workspaceFolders;
      this.workspaceRoot = folders && folders.length > 0 ? folders[0].uri.fsPath : process.cwd();
    }
  }

  public setWorkspaceRoot(root: string): void {
    this.workspaceRoot = root;
  }

  public getWorkspaceRoot(): string {
    return this.workspaceRoot;
  }

  /**
   * Resolves the cosm binary invocation.
   * Checks VS Code settings, repository build paths, and falls back to PATH.
   */
  public getExecutableCommand(): { command: string; prefixArgs: string[] } {
    const config = vscode.workspace.getConfiguration('cosm');
    const binaryConfig = config.get<string>('binaryPath', 'cosm');

    // 1. Direct configured binary or absolute path
    if (binaryConfig && binaryConfig !== 'cosm') {
      if (path.isAbsolute(binaryConfig) && fs.existsSync(binaryConfig)) {
        return { command: binaryConfig, prefixArgs: [] };
      }
      const resolved = path.resolve(this.workspaceRoot, binaryConfig);
      if (fs.existsSync(resolved)) {
        return { command: resolved, prefixArgs: [] };
      }
    }

    // 2. Check for built binary in repository root or bin/
    const candidatePaths = [
      path.join(this.workspaceRoot, 'bin', 'cosm'),
      path.join(this.workspaceRoot, 'cosm'),
      path.resolve(this.workspaceRoot, '..', '..', 'bin', 'cosm'),
      path.resolve(this.workspaceRoot, '..', '..', 'cosm'),
      path.resolve(this.workspaceRoot, '..', 'bin', 'cosm'),
      path.resolve(this.workspaceRoot, '..', 'cosm'),
    ];
    for (const cand of candidatePaths) {
      if (fs.existsSync(cand)) {
        return { command: cand, prefixArgs: [] };
      }
    }

    // 4. If Go is available and cmd/cosm exists, allow running via 'go run ./cmd/cosm'
    const cmdCosmDir = path.join(this.workspaceRoot, 'cmd', 'cosm');
    const parentCmdCosmDir = path.resolve(this.workspaceRoot, '..', '..', 'cmd', 'cosm');
    if (fs.existsSync(cmdCosmDir)) {
      return { command: 'go', prefixArgs: ['run', './cmd/cosm'] };
    } else if (fs.existsSync(parentCmdCosmDir)) {
      return { command: 'go', prefixArgs: ['run', './cmd/cosm'] };
    }

    // 5. Default to system PATH 'cosm'
    return { command: binaryConfig || 'cosm', prefixArgs: [] };
  }

  /**
   * Executes a Cosm command with standard input/output capture.
   */
  public async execCosm(args: string[]): Promise<string> {
    const { command, prefixArgs } = this.getExecutableCommand();
    const fullArgs = [...prefixArgs, ...args];

    return new Promise((resolve, reject) => {
      cp.execFile(
        command,
        fullArgs,
        {
          cwd: this.workspaceRoot,
          maxBuffer: 10 * 1024 * 1024,
          env: { ...process.env, PAGER: 'cat' }
        },
        (error, stdout, stderr) => {
          if (error) {
            const msg = stderr ? stderr.trim() : error.message;
            return reject(new Error(`cosm ${args[0]} failed: ${msg}`));
          }
          resolve(stdout.trim());
        }
      );
    });
  }

  /**
   * Retrieves active repository status, active universe, Merkle root, and component count.
   */
  public async getStatus(universe?: string): Promise<CosmStatus> {
    const args = ['status', '-format', 'json'];
    if (universe) {
      args.push('-u', universe);
    }

    try {
      const output = await this.execCosm(args);
      const jsonStart = output.indexOf('{');
      if (jsonStart !== -1) {
        const parsed = JSON.parse(output.substring(jsonStart));
        return {
          status: parsed.status || 'SUCCESS',
          universe_id: parsed.universe_id || universe || 'universe-main',
          merkle_root: parsed.merkle_root || '',
          components_count: parsed.components_count || 0,
          cross_edges_count: parsed.cross_edges_count || 0,
          components: parsed.components || [],
          clean: parsed.working_tree_lens?.status === 'clean',
          working_tree_lens: parsed.working_tree_lens,
          staged_files: parsed.staged_files,
          modified_files: parsed.modified_files
        };
      }
    } catch {
      // Fallback to text parsing
      const rawText = await this.execCosm(universe ? ['status', '-u', universe] : ['status']);
      return this.parseTextStatus(rawText, universe);
    }

    return {
      status: 'SUCCESS',
      universe_id: universe || 'universe-main',
      merkle_root: '',
      components_count: 0,
      cross_edges_count: 0,
      components: []
    };
  }

  private parseTextStatus(text: string, defaultUniverse?: string): CosmStatus {
    const universeMatch = text.match(/On universe:\s*([^\n\r]+)/);
    const merkleMatch = text.match(/Head Merkle Root:\s*([^\n\r]+)/);
    const compMatch = text.match(/Components tracked:\s*(\d+)/);
    const edgeMatch = text.match(/Cross-boundary edges:\s*(\d+)/);

    return {
      status: 'SUCCESS',
      universe_id: universeMatch ? universeMatch[1].trim() : defaultUniverse || 'universe-main',
      merkle_root: merkleMatch ? merkleMatch[1].trim() : '',
      components_count: compMatch ? parseInt(compMatch[1], 10) : 0,
      cross_edges_count: edgeMatch ? parseInt(edgeMatch[1], 10) : 0,
      components: []
    };
  }

  /**
   * Lists all active micro-universes in the workspace.
   */
  public async listUniverses(): Promise<Array<{ universe_id: string; head_manifest_hash: string; status: string }>> {
    try {
      const output = await this.execCosm(['universe', 'list', '-format', 'json']);
      const jsonStart = output.indexOf('[');
      if (jsonStart !== -1) {
        const parsed = JSON.parse(output.substring(jsonStart));
        if (Array.isArray(parsed)) {
          return parsed.map((item: any) => ({
            universe_id: item.universe_id || item.UniverseID || '',
            head_manifest_hash: item.head_manifest_hash || item.HeadManifestHash || '',
            status: item.status || item.Status || 'active'
          }));
        }
      }
    } catch {
      // Fallback to text parsing
    }

    const text = await this.execCosm(['universe', 'list']);
    const list: Array<{ universe_id: string; head_manifest_hash: string; status: string }> = [];
    const lines = text.split('\n');
    for (const line of lines) {
      const match = line.match(/\*\s*([^\s]+)\s*\(Head:\s*([^,\)]+)(?:,\s*Status:\s*([^\)]+))?\)/);
      if (match) {
        list.push({
          universe_id: match[1].trim(),
          head_manifest_hash: match[2].trim(),
          status: (match[3] || 'active').trim()
        });
      }
    }
    return list;
  }

  /**
   * Switches active micro-universe in Cosm CLI.
   */
  public async switchUniverse(universeId: string): Promise<string> {
    return this.execCosm(['universe', 'switch', universeId]);
  }

  /**
   * Merges a source micro-universe into a target micro-universe.
   */
  public async mergeUniverse(
    sourceUniverse: string,
    targetUniverse: string = 'universe-main',
    strategy: string = 'union'
  ): Promise<{ target_universe: string; head_hash: string }> {
    const output = await this.execCosm([
      'universe',
      'merge',
      sourceUniverse,
      '-t',
      targetUniverse,
      '-s',
      strategy
    ]);
    const headMatch = output.match(/Head:\s*([a-f0-9]+)/i);
    const targetMatch = output.match(/into\s*['"]([^'"]+)['"]/i);
    return {
      target_universe: targetMatch ? targetMatch[1] : targetUniverse,
      head_hash: headMatch ? headMatch[1] : ''
    };
  }

  /**
   * Retrieves commit log history for a universe as structured CommitLogEntry array.
   */
  public async getLog(universe?: string, limit: number = 20): Promise<CommitLogEntry[]> {
    const args = ['log', ...(universe ? ['-u', universe] : []), '-n', limit.toString(), '-format', 'json'];
    try {
      const output = await this.execCosm(args);
      const jsonStart = output.indexOf('[');
      if (jsonStart !== -1) {
        return JSON.parse(output.substring(jsonStart)) as CommitLogEntry[];
      }
    } catch {
      // Fallback: return empty array on failure
    }
    return [];
  }

  /**
   * Creates a new zero-copy micro-universe branched from a parent.
   */
  public async createUniverse(id: string, parent: string = 'universe-main'): Promise<{ universe_id: string; head_hash: string }> {
    const output = await this.execCosm(['universe', 'create', '-p', parent, id]);
    const match = output.match(/Head:\s*([a-f0-9]+)/i);
    return {
      universe_id: id,
      head_hash: match ? match[1] : ''
    };
  }

  /**
   * Stages one or more polyglot files/directories into AST symbols.
   */
  public async stageFiles(files: string[], intent?: string, prompt?: string, universe?: string): Promise<string> {
    const args = ['add', ...files];
    if (intent) {
      args.push('--intent', intent);
    }
    if (prompt) {
      args.push('--prompt', prompt);
    }
    args.push('--allow-secrets');
    if (universe) {
      args.push('-u', universe);
    }
    return this.execCosm(args);
  }

  /**
   * Commits staged AST symbols with causal lineage and updates universe head.
   */
  public async commit(options: CommitOptions): Promise<{ merkle_root: string; message: string }> {
    const args = ['commit', '-i', options.intent];
    if (options.prompt && options.prompt.trim().length > 0) {
      args.push('-p', options.prompt);
    }
    if (options.universe) {
      args.push('-u', options.universe);
    }
    if (options.agent) {
      args.push('-a', options.agent);
    }
    if (options.model) {
      args.push('-m', options.model);
    }
    if (options.sessionId) {
      args.push('--session-id', options.sessionId);
    }

    const output = await this.execCosm(args);
    const merkleMatch = output.match(/Merkle Root:\s*([a-f0-9]+)/i);

    return {
      merkle_root: merkleMatch ? merkleMatch[1] : '',
      message: output
    };
  }

  /**
   * Resolves an AST symbol by identifier or file target using 'cosm ast resolve'.
   */
  public async resolveSymbol(target: string, universe?: string): Promise<ResolvedSymbol | null> {
    const args = ['ast', 'resolve', '-format', 'json', target];
    if (universe) {
      args.push('-u', universe);
    }

    try {
      const output = await this.execCosm(args);
      const jsonStart = output.indexOf('{');
      if (jsonStart !== -1) {
        return JSON.parse(output.substring(jsonStart)) as ResolvedSymbol;
      }
    } catch (err) {
      // Symbol resolution may return non-zero if symbol doesn't exist
      return null;
    }
    return null;
  }

  /**
   * Retrieves the 3-tier architectural topology graph (Frontend -> Backend -> Cloud Infra).
   */
  public async getTopology(universe?: string): Promise<FullTopologyGraph> {
    const args = ['topology', '-format', 'json'];
    if (universe) {
      args.push('-u', universe);
    }

    try {
      const output = await this.execCosm(args);
      const jsonStart = output.indexOf('{');
      if (jsonStart !== -1) {
        return JSON.parse(output.substring(jsonStart)) as FullTopologyGraph;
      }
    } catch {
      // Fallback
    }

    return {
      frontend_nodes: [],
      backend_nodes: [],
      infra_nodes: [],
      total_nodes: 0,
      total_edges: 0
    };
  }

  /**
   * Retrieves the comprehensive AST Tree and symbol dependency graph.
   * Invokes 'cosm ast tree --format json' and falls back to synthesizing
   * an ASTTreeGraph from getStatus() and getTopology() when the CLI call fails
   * or returns non-JSON (e.g. older binary).
   */
  public async getASTTree(universe?: string): Promise<ASTTreeGraph> {
    const args = ['ast', 'tree', '--format', 'json', ...(universe ? ['-u', universe] : [])];

    try {
      const output = await this.execCosm(args);
      const jsonStart = output.indexOf('{');
      if (jsonStart !== -1) {
        const parsed = JSON.parse(output.substring(jsonStart)) as ASTTreeGraph;
        if (parsed && Array.isArray(parsed.components)) {
          return {
            universe_id: parsed.universe_id || universe || 'universe-main',
            merkle_root: parsed.merkle_root || '',
            total_components: parsed.total_components ?? parsed.components.length,
            total_symbols: parsed.total_symbols ?? parsed.components.reduce((acc, c) => acc + (c.symbols ? c.symbols.length : 0), 0),
            total_edges: parsed.total_edges ?? 0,
            components: parsed.components
          };
        }
      }
    } catch {
      // CLI call failed or returned non-JSON -> fallback resilience
    }

    return this.synthesizeASTTreeFromFallback(universe);
  }

  /**
   * Synthesizes an ASTTreeGraph from getStatus() and getTopology() for backward compatibility.
   */
  private async synthesizeASTTreeFromFallback(universe?: string): Promise<ASTTreeGraph> {
    const [status, topology] = await Promise.all([
      this.getStatus(universe),
      this.getTopology(universe)
    ]);

    const universeId = status.universe_id || universe || 'universe-main';
    const merkleRoot = status.merkle_root || '';

    const componentMap = new Map<string, ASTTreeComponent>();
    let edgeCount = 0;

    const allNodes: TopologyNode[] = [
      ...(topology.frontend_nodes || []),
      ...(topology.backend_nodes || []),
      ...(topology.infra_nodes || [])
    ];

    for (const node of allNodes) {
      let compName = node.name;
      let symbolName = node.name;

      if (node.name.includes('::')) {
        const idx = node.name.indexOf('::');
        compName = node.name.substring(0, idx);
        symbolName = node.name.substring(idx + 2);
      } else if (node.name.includes(':')) {
        const idx = node.name.indexOf(':');
        compName = node.name.substring(0, idx);
        symbolName = node.name.substring(idx + 1);
      }

      const outgoingEdges: ASTTreeEdge[] = (node.outgoing || []).map(e => ({
        target_id: e.target_id,
        edge_type: e.edge_type,
        label: e.label
      }));

      edgeCount += outgoingEdges.length;

      const symbol: ASTTreeSymbol = {
        node_id: node.id,
        identifier: symbolName,
        node_type: node.type || 'Symbol',
        language: node.language || '',
        outgoing_edges: outgoingEdges
      };

      if (!componentMap.has(compName)) {
        componentMap.set(compName, {
          component_id: compName,
          name: compName,
          language: node.language || '',
          type: node.tier || 'Component',
          symbols: []
        });
      }

      componentMap.get(compName)!.symbols.push(symbol);
    }

    if (status.components && Array.isArray(status.components)) {
      for (const comp of status.components) {
        if (!componentMap.has(comp)) {
          componentMap.set(comp, {
            component_id: comp,
            name: comp,
            language: '',
            type: 'Component',
            symbols: []
          });
        }
      }
    }

    const components = Array.from(componentMap.values());
    const totalSymbols = components.reduce((sum, c) => sum + c.symbols.length, 0);

    return {
      universe_id: universeId,
      merkle_root: merkleRoot,
      total_components: Math.max(components.length, status.components_count || 0),
      total_symbols: totalSymbols,
      total_edges: Math.max(edgeCount, topology.total_edges || 0, status.cross_edges_count || 0),
      components
    };
  }


  /**
   * Runs blast radius audit for an agent, symbol, or model modification.
   */
  public async getBlastRadius(target: string): Promise<BlastRadiusReport> {
    const args = ['blast-radius', '-format', 'json', target];

    try {
      const output = await this.execCosm(args);
      const jsonStart = output.indexOf('{');
      if (jsonStart !== -1) {
        const parsed = JSON.parse(output.substring(jsonStart));
        const incoming = parsed.incoming_edges || [];
        const outgoing = parsed.outgoing_edges || [];
        const direct = parsed.total_direct_nodes ?? parsed.TotalDirectNodes ?? (incoming.length + outgoing.length);
        const affected = parsed.affected_components || [];
        const downstream = parsed.total_downstream_nodes ?? parsed.TotalDownstreamNodes ?? affected.length;
        const targetId = parsed.target_symbol_id || target;
        const risk = typeof parsed.risk_score === 'number' ? parsed.risk_score : (typeof parsed.RiskScore === 'number' ? parsed.RiskScore : 0.0);
        const summary = parsed.summary || parsed.Summary || `Blast radius impact analysis for ${targetId} (${direct} direct contracts, ${downstream} downstream components)`;

        return {
          ...parsed,
          summary,
          total_direct_nodes: direct,
          total_downstream_nodes: downstream,
          risk_score: risk,
          target_symbol_id: targetId,
          incoming_edges: incoming,
          outgoing_edges: outgoing,
          affected_components: affected,
          warnings: parsed.warnings || []
        };
      }
    } catch {
      // Fallback text parsing
    }

    try {
      const text = await this.execCosm(['blast-radius', target]);
      const dirMatch = text.match(/Total Direct Nodes:\s*(\d+)/);
      const downMatch = text.match(/Total Downstream Nodes:\s*(\d+)/);
      const riskMatch = text.match(/Risk Score:\s*([\d\.]+)/);
      const sumMatch = text.match(/Summary:\s*([^\n\r]+)/);

      return {
        summary: sumMatch ? sumMatch[1].trim() : `Blast radius for ${target}`,
        total_direct_nodes: dirMatch ? parseInt(dirMatch[1], 10) : 0,
        total_downstream_nodes: downMatch ? parseInt(downMatch[1], 10) : 0,
        risk_score: riskMatch ? parseFloat(riskMatch[1]) : 0.0
      };
    } catch (e: any) {
      return {
        summary: `Audit error: ${e.message}`,
        total_direct_nodes: 0,
        total_downstream_nodes: 0,
        risk_score: 0.0
      };
    }
  }

  /**
   * Executes autonomous shipping sidecar ('cosm ship') for ephemeral preview build.
   */
  public async shipPreview(universe?: string, targetProfile?: string): Promise<ShipResult> {
    const args = ['ship', '-format', 'json'];
    if (universe) {
      args.push('-u', universe);
    }
    if (targetProfile) {
      args.push('--target', targetProfile);
    }

    try {
      const output = await this.execCosm(args);
      const jsonStart = output.indexOf('{');
      if (jsonStart !== -1) {
        return JSON.parse(output.substring(jsonStart)) as ShipResult;
      }
    } catch {
      // Fallback text parsing
    }

    const text = await this.execCosm(args.filter(a => a !== '-format' && a !== 'json'));
    const artIdMatch = text.match(/Artifact ID:\s*([^\n\r]+)/);
    const pathMatch = text.match(/Artifact Path:\s*([^\n\r]+)/);
    const sizeMatch = text.match(/Size:\s*(\d+)\s*bytes/);
    const urlMatch = text.match(/Preview URL:\s*([^\n\r]+)/);
    const healthMatch = text.match(/Health:\s*(true|false)/i);

    return {
      status: 'SHIPPED',
      target: targetProfile || 'local-preview',
      universe_id: universe || 'universe-main',
      merkle_root: '',
      artifact_id: artIdMatch ? artIdMatch[1].trim() : 'art-preview',
      artifact_path: pathMatch ? pathMatch[1].trim() : '',
      size_bytes: sizeMatch ? parseInt(sizeMatch[1], 10) : 0,
      preview_url: urlMatch ? urlMatch[1].trim() : 'http://127.0.0.1:8080/preview',
      healthy: healthMatch ? healthMatch[1].toLowerCase() === 'true' : true
    };
  }

  /**
   * Creates a Jujutsu-style stacked proposal.
   */
  public async createStack(options: { changeId: string; universeId: string; parentId?: string; title?: string }): Promise<string> {
    const args = ['stack', 'create', '-c', options.changeId, '-u', options.universeId];
    if (options.parentId) {
      args.push('-p', options.parentId);
    }
    if (options.title) {
      args.push('--title', options.title);
    }
    return this.execCosm(args);
  }

  /**
   * Evolves/rebases descendants in a proposal stack.
   */
  public async evolveStack(parentChangeId: string): Promise<string> {
    return this.execCosm(['stack', 'evolve', '-c', parentChangeId]);
  }

  /**
   * Retrieves stacked proposals (Jujutsu-style stacked changes).
   */
  public async listStack(): Promise<StackRecord[]> {
    let output = '';
    try {
      output = await this.execCosm(['stack', 'list', '-format', 'json']);
      const trimmed = output.trim();
      if (trimmed.startsWith('[') || trimmed.startsWith('{')) {
        const jsonStart = trimmed.search(/[\[{]/);
        const parsed = JSON.parse(trimmed.substring(jsonStart));
        if (Array.isArray(parsed)) {
          return parsed as StackRecord[];
        } else if (parsed && typeof parsed === 'object') {
          return [parsed as StackRecord];
        }
      }
    } catch {
      // JSON attempt failed, fallback to text parsing
    }

    try {
      const text = output && !output.trim().startsWith('[') && !output.trim().startsWith('{')
        ? output
        : await this.execCosm(['stack', 'list']);
      return this.parseTextStack(text);
    } catch {
      return [];
    }
  }

  private parseTextStack(text: string): StackRecord[] {
    const records: StackRecord[] = [];
    const lines = text.split(/\r?\n/);
    let currentRecord: Partial<StackRecord> | null = null;

    const headerRegex = /\[(\d+)\]\s*🔹\s*([^\s]+)\s*\(Universe:\s*([^,]+),\s*Head:\s*([^\)]+)\)/;
    const parentRegex = /Parent:\s*([^\s|]+)/;

    for (const line of lines) {
      const headerMatch = line.match(headerRegex);
      if (headerMatch) {
        if (currentRecord && currentRecord.change_id && currentRecord.universe_id) {
          records.push(currentRecord as StackRecord);
        }
        currentRecord = {
          order_index: parseInt(headerMatch[1], 10),
          change_id: headerMatch[2].trim(),
          universe_id: headerMatch[3].trim(),
          manifest_hash: headerMatch[4].trim()
        };
        continue;
      }

      if (currentRecord) {
        const parentMatch = line.match(parentRegex);
        if (parentMatch) {
          currentRecord.parent_change_id = parentMatch[1].trim();
        }
      }
    }

    if (currentRecord && currentRecord.change_id && currentRecord.universe_id) {
      records.push(currentRecord as StackRecord);
    }

    return records;
  }

  /**
   * Views reconstituted source or AST definition for a symbol node.
   */
  public async viewNode(nodeId: string, format: 'source' | 'ast' | 'raw' = 'source'): Promise<string> {
    return this.execCosm(['view', nodeId, '--format', format]);
  }
}
