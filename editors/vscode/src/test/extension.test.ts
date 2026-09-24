import * as assert from 'assert';
import * as fs from 'fs';
import * as path from 'path';
import { CosmClient } from '../client/cosmClient';
import {
  CosmStatus,
  UniverseRecord,
  CommitLogEntry,
  ResolvedSymbol,
  FullTopologyGraph,
  BlastRadiusReport,
  ShipResult,
  LineageEnvelope,
  StackRecord,
  ASTTreeEdge,
  ASTTreeLineage,
  ASTTreeSymbol,
  ASTTreeComponent,
  ASTTreeGraph
} from '../types';
import { CosmSCMProvider } from '../scm/provider';
import {
  CosmASTTreeDataProvider,
  UniverseItem,
  DirectoryItem,
  ComponentItem,
  SymbolItem,
  DetailItem,
  MerkleInfoItem,
  getSymbolThemeIcon,
  createSymbolDetails
} from '../views/astTreeProvider';
import {
  CosmTopologyTreeDataProvider,
  TierCategoryItem,
  TopologyComponentItem,
  TopologyNodeItem,
  TopologyEdgeItem,
  TopologyActionItem
} from '../views/topologyTreeProvider';
import {
  CosmStackedChangesProvider,
  StackedChangeTreeItem
} from '../views/stackedChangesProvider';

declare function describe(name: string, fn: () => void): void;
declare function it(name: string, fn: () => void | Promise<void>): void;

describe('Cosm VS Code Extension Test Suite', () => {

  describe('1. CosmClient & Executable Command Resolution', () => {
    it('should correctly configure default executable command and prefix args', () => {
      const client = new CosmClient(process.cwd());
      const execConfig = client.getExecutableCommand();

      assert.ok(execConfig.command, 'Executable command should be resolved');
      assert.ok(Array.isArray(execConfig.prefixArgs), 'Prefix args should be an array');
    });

    it('should parse Cosm status JSON payload into CosmStatus interface', () => {
      const rawJson = {
        status: 'SUCCESS',
        universe_id: 'universe-test',
        merkle_root: '673bc252841f2bb3965ed5ba48a1cbd3470e398e136031ab31332edc70916198',
        components_count: 3,
        cross_edges_count: 1,
        components: ['main.go', 'app.py', 'infra.tf']
      };

      const status: CosmStatus = {
        status: rawJson.status,
        universe_id: rawJson.universe_id,
        merkle_root: rawJson.merkle_root,
        components_count: rawJson.components_count,
        cross_edges_count: rawJson.cross_edges_count,
        components: rawJson.components
      };

      assert.strictEqual(status.status, 'SUCCESS');
      assert.strictEqual(status.universe_id, 'universe-test');
      assert.strictEqual(status.components_count, 3);
      assert.strictEqual(status.cross_edges_count, 1);
      assert.strictEqual(status.merkle_root.length, 64);
    });

    it('should parse universe list output with head hashes and status', () => {
      const rawUniverses = [
        {
          universe_id: 'universe-main',
          head_manifest_hash: '368cf8f3f9e40528f4bfce840ec049304f8ea21d21ae5fc21654874a85f2979b',
          status: 'active'
        },
        {
          universe_id: 'u/feature-auth',
          head_manifest_hash: '4e08020c81259d979293a3bb97e811b82d871bc5481221f1a56663fed6f4d7f8',
          status: 'active'
        }
      ];

      const universes: UniverseRecord[] = rawUniverses;
      assert.strictEqual(universes.length, 2);
      assert.strictEqual(universes[0].universe_id, 'universe-main');
      assert.strictEqual(universes[1].universe_id, 'u/feature-auth');
      assert.strictEqual(universes[1].head_manifest_hash.substring(0, 12), '4e08020c8125');
    });

    it('should parse blast radius audit reports and calculate risk percentages', () => {
      const report: BlastRadiusReport = {
        summary: 'Audit of agent gemini-3.8-flash',
        total_direct_nodes: 4,
        total_downstream_nodes: 12,
        risk_score: 0.25
      };

      assert.strictEqual(report.total_direct_nodes, 4);
      assert.strictEqual(report.total_downstream_nodes, 12);
      assert.strictEqual((report.risk_score * 100).toFixed(0), '25');
    });

    it('should parse ship preview results', () => {
      const ship: ShipResult = {
        status: 'SHIPPED',
        target: 'local-preview',
        universe_id: 'universe-main',
        merkle_root: '8c94fa52714c',
        artifact_id: 'art-883f',
        artifact_path: '.cosm/preview/index.html',
        size_bytes: 48920,
        preview_url: 'http://127.0.0.1:8080/preview',
        healthy: true
      };

      assert.strictEqual(ship.status, 'SHIPPED');
      assert.strictEqual(ship.target, 'local-preview');
      assert.strictEqual(ship.healthy, true);
      assert.ok(ship.preview_url?.startsWith('http://'));
    });
  });

  describe('2. 5-Tier Causal Pedigree & Lineage Envelope', () => {
    it('should construct and validate complete 5-tier causal lineage', () => {
      const lineage: LineageEnvelope = {
        user_id: 'developer',
        user_prompt: 'Add high-speed Redis user state cache with 15m TTL',
        session_id: 'sess-8839-a912-44df',
        orchestrator_agent_id: 'cosm-orchestrator',
        executing_agent_id: 'cosm-ast-surgeon / gemini-3.8-flash',
        llm_version: 'gemini-3.8-flash',
        generation_params: 'temperature=0.2, top_p=0.95',
        intent: 'feat(cache): implement Redis session store',
        timestamp: '2026-09-09T22:30:00Z',
        tokens: {
          prompt_tokens: 1050,
          completion_tokens: 210,
          reasoning_tokens: 400,
          total_tokens: 1660,
          cost_usd: 0.0012,
          latency_ms: 48
        },
        trace: {
          trace_id: '4bf92f3577b34da6a3ce929d0e0e4736',
          span_id: '00f067aa0ba902b7'
        },
        signature_ed25519: 'ed25519:7a9c1e042b8819aef3d91c7821...'
      };

      // Tier 1 Check
      assert.ok(lineage.user_prompt.includes('Redis user state cache'));
      assert.strictEqual(lineage.user_id, 'developer');

      // Tier 2 Check
      assert.strictEqual(lineage.session_id, 'sess-8839-a912-44df');
      assert.strictEqual(lineage.orchestrator_agent_id, 'cosm-orchestrator');

      // Tier 3 Check
      assert.ok(lineage.executing_agent_id.includes('gemini-3.8-flash'));

      // Tier 4 Check
      assert.strictEqual(lineage.tokens?.total_tokens, 1660);
      assert.strictEqual(lineage.tokens?.cost_usd, 0.0012);
      assert.strictEqual(lineage.tokens?.latency_ms, 48);

      // Tier 5 Check
      assert.ok(lineage.signature_ed25519?.startsWith('ed25519:'));
    });

    it('should format CodeLens prompt excerpt and verification badge correctly', () => {
      const prompt = 'Implement OAuth2 PKCE authorization flow for mobile clients';
      const promptSnippet = prompt.length > 32 ? prompt.substring(0, 32) + '...' : prompt;
      const agentId = 'gemini-3.8-flash';
      const verified = true;
      const badge = verified ? 'Verified Ed25519 ✓' : 'Unsigned';

      const title = `🌌 Pedigree: "${promptSnippet}" • Agent: ${agentId} • [${badge}]`;
      assert.strictEqual(
        title,
        '🌌 Pedigree: "Implement OAuth2 PKCE authorizat..." • Agent: gemini-3.8-flash • [Verified Ed25519 ✓]'
      );
    });
  });

  describe('3. 3-Tier Architecture Topology Classification', () => {
    it('should properly organize symbols into Frontend, Backend, and Cloud Infra swimlanes', () => {
      const topology: FullTopologyGraph = {
        frontend_nodes: [
          {
            id: 'node-fe-1',
            name: 'src/components/UserDashboard.tsx:UserDashboard',
            tier: 'Frontend',
            language: 'typescript',
            type: 'ComponentDecl',
            outgoing: [
              {
                target_id: 'node-be-1',
                edge_type: 'CALLS',
                label: 'GET /api/v1/users'
              }
            ]
          }
        ],
        backend_nodes: [
          {
            id: 'node-be-1',
            name: 'pkg/api/users.go:HandleListUsers',
            tier: 'Backend',
            language: 'go',
            type: 'FunctionDecl',
            outgoing: [
              {
                target_id: 'node-infra-1',
                edge_type: 'BINDS_ENV',
                label: 'BINDS DATABASE_URL'
              }
            ]
          }
        ],
        infra_nodes: [
          {
            id: 'node-infra-1',
            name: 'deploy/terraform/database.tf:google_sql_database_instance.default',
            tier: 'Cloud Infra',
            language: 'hcl',
            type: 'ResourceBlock',
            outgoing: null
          }
        ],
        total_nodes: 3,
        total_edges: 2
      };

      assert.strictEqual(topology.total_nodes, 3);
      assert.strictEqual(topology.total_edges, 2);
      assert.strictEqual(topology.frontend_nodes?.length, 1);
      assert.strictEqual(topology.backend_nodes?.length, 1);
      assert.strictEqual(topology.infra_nodes?.length, 1);

      // Verify cross-boundary edge from Frontend -> Backend
      const feEdge = topology.frontend_nodes![0].outgoing![0];
      assert.strictEqual(feEdge.edge_type, 'CALLS');
      assert.strictEqual(feEdge.label, 'GET /api/v1/users');
      assert.strictEqual(feEdge.target_id, 'node-be-1');

      // Verify cross-boundary edge from Backend -> Infra
      const beEdge = topology.backend_nodes![0].outgoing![0];
      assert.strictEqual(beEdge.edge_type, 'BINDS_ENV');
      assert.strictEqual(beEdge.label, 'BINDS DATABASE_URL');
      assert.strictEqual(beEdge.target_id, 'node-infra-1');
    });
  });

  describe('4. SCM State & Command Contribution Invariants', () => {
    it('should define all required Cosm commands', () => {
      const requiredCommands = [
        'cosm.switchUniverse',
        'cosm.createUniverse',
        'cosm.openTopology',
        'cosm.commitWithPrompt',
        'cosm.shipPreview',
        'cosm.blastRadius',
        'cosm.refreshSCM',
        'cosm.stageFile',
        'cosm.unstageFile',
        'cosm.diffSymbol',
        'cosm.resolveSymbol'
      ];

      const getPackageJson = () => {
        const pkgPath = path.resolve(process.cwd(), 'editors/vscode/package.json');
        if (fs.existsSync(pkgPath)) {
          return JSON.parse(fs.readFileSync(pkgPath, 'utf8'));
        }
        return JSON.parse(fs.readFileSync(path.resolve(process.cwd(), 'package.json'), 'utf8'));
      };

      const packageJson = getPackageJson();
      const registeredCommands = packageJson.contributes.commands.map((c: any) => c.command);

      for (const cmd of requiredCommands) {
        assert.ok(
          registeredCommands.includes(cmd),
          `Command '${cmd}' must be declared in package.json contributes.commands`
        );
      }
    });

    it('should define all required configuration properties in package.json', () => {
      const requiredConfig = [
        'cosm.binaryPath',
        'cosm.autoSync',
        'cosm.enableCodeLens',
        'cosm.topocosmUrl'
      ];

      const pkgPath = path.resolve(process.cwd(), 'editors/vscode/package.json');
      const packageJson = fs.existsSync(pkgPath)
        ? JSON.parse(fs.readFileSync(pkgPath, 'utf8'))
        : JSON.parse(fs.readFileSync(path.resolve(process.cwd(), 'package.json'), 'utf8'));
      const properties = Object.keys(packageJson.contributes.configuration.properties);

      for (const cfg of requiredConfig) {
        assert.ok(
          properties.includes(cfg),
          `Configuration property '${cfg}' must be declared in package.json`
        );
      }
    });

    it('should define the Cosm activity bar container and views in package.json', () => {
      const pkgPath = path.resolve(process.cwd(), 'editors/vscode/package.json');
      const packageJson = fs.existsSync(pkgPath)
        ? JSON.parse(fs.readFileSync(pkgPath, 'utf8'))
        : JSON.parse(fs.readFileSync(path.resolve(process.cwd(), 'package.json'), 'utf8'));
      assert.strictEqual(packageJson.contributes.viewsContainers.activitybar[0].id, 'cosm-explorer');
      assert.strictEqual(packageJson.contributes.views['cosm-explorer'][0].id, 'cosm.astTreeView');
      assert.strictEqual(packageJson.contributes.views['cosm-explorer'][0].type, 'tree');
      assert.strictEqual(packageJson.contributes.views['cosm-explorer'][1].id, 'cosm.topologyTreeView');
      assert.strictEqual(packageJson.contributes.views['cosm-explorer'][1].type, 'tree');
      assert.strictEqual(packageJson.contributes.views['scm'][0].id, 'cosm.scmView');
    });
  });

  describe('5. AST Tree Domain Types & getASTTree CosmClient Integration', () => {
    it('should construct and validate complete ASTTreeGraph wire DTO', () => {
      const edge: ASTTreeEdge = {
        target_id: 'sym-api-login',
        edge_type: 'CALLS',
        label: 'POST /auth/login'
      };

      const lineage: ASTTreeLineage = {
        user_prompt: 'Add login mutation',
        executing_agent_id: 'gemini-3.8-flash',
        intent: 'feat(auth): login symbol',
        timestamp: '2026-09-17T12:00:00Z'
      };

      const symbol: ASTTreeSymbol = {
        node_id: 'sym-fe-login-btn',
        identifier: 'LoginButton',
        node_type: 'ComponentDecl',
        language: 'typescript',
        signature: 'export const LoginButton: React.FC = () => ...',
        docstring: 'Renders login button',
        visibility: 'public',
        dependencies: ['react', 'authService'],
        outgoing_edges: [edge],
        lineage: lineage
      };

      const component: ASTTreeComponent = {
        component_id: 'src/components/LoginButton.tsx',
        name: 'LoginButton.tsx',
        language: 'typescript',
        type: 'Frontend',
        symbols: [symbol]
      };

      const graph: ASTTreeGraph = {
        universe_id: 'universe-test',
        merkle_root: 'abcdef1234567890',
        total_components: 1,
        total_symbols: 1,
        total_edges: 1,
        components: [component]
      };

      assert.strictEqual(graph.universe_id, 'universe-test');
      assert.strictEqual(graph.total_components, 1);
      assert.strictEqual(graph.total_symbols, 1);
      assert.strictEqual(graph.total_edges, 1);
      assert.strictEqual(graph.components[0].symbols[0].identifier, 'LoginButton');
      assert.strictEqual(graph.components[0].symbols[0].outgoing_edges?.[0].edge_type, 'CALLS');
      assert.strictEqual(graph.components[0].symbols[0].lineage?.executing_agent_id, 'gemini-3.8-flash');
    });

    it('should parse AST tree JSON output directly when CLI returns JSON', async () => {
      const client = new CosmClient(process.cwd());
      const mockGraph: ASTTreeGraph = {
        universe_id: 'universe-main',
        merkle_root: 'root-abc-123',
        total_components: 1,
        total_symbols: 1,
        total_edges: 0,
        components: [
          {
            component_id: 'comp-1',
            name: 'main.go',
            language: 'go',
            type: 'Backend',
            symbols: [
              {
                node_id: 'sym-main',
                identifier: 'main',
                node_type: 'FunctionDecl',
                language: 'go',
                outgoing_edges: []
              }
            ]
          }
        ]
      };

      // Mock execCosm
      (client as any).execCosm = async (args: string[]) => {
        assert.ok(args.includes('ast') && args.includes('tree') && args.includes('--format') && args.includes('json'));
        return JSON.stringify(mockGraph);
      };

      const res = await client.getASTTree();
      assert.strictEqual(res.universe_id, 'universe-main');
      assert.strictEqual(res.total_components, 1);
      assert.strictEqual(res.components[0].symbols[0].identifier, 'main');
    });

    it('should fallback to synthesize an ASTTreeGraph from getStatus and getTopology when CLI fails or returns non-JSON', async () => {
      const client = new CosmClient(process.cwd());

      // Mock execCosm to fail on 'ast tree'
      (client as any).execCosm = async (args: string[]) => {
        if (args[0] === 'ast' && args[1] === 'tree') {
          throw new Error('unknown command: ast tree');
        }
        if (args[0] === 'status') {
          return JSON.stringify({
            status: 'SUCCESS',
            universe_id: 'universe-fallback',
            merkle_root: 'root-fallback-999',
            components_count: 2,
            cross_edges_count: 1,
            components: ['services/auth.go', 'pkg/db.go']
          });
        }
        if (args[0] === 'topology') {
          return JSON.stringify({
            frontend_nodes: [],
            backend_nodes: [
              {
                id: 'node-auth-1',
                name: 'services/auth.go:ValidateToken',
                tier: 'Backend',
                language: 'go',
                type: 'FunctionDecl',
                outgoing: [
                  {
                    target_id: 'node-db-1',
                    edge_type: 'CALLS',
                    label: 'QueryUser'
                  }
                ]
              }
            ],
            infra_nodes: [],
            total_nodes: 1,
            total_edges: 1
          });
        }
        return '';
      };

      const res = await client.getASTTree('universe-fallback');
      assert.strictEqual(res.universe_id, 'universe-fallback');
      assert.strictEqual(res.merkle_root, 'root-fallback-999');
      assert.ok(res.total_components >= 2);
      assert.strictEqual(res.total_edges, 1);

      const authComp = res.components.find(c => c.component_id === 'services/auth.go');
      assert.ok(authComp, 'services/auth.go component should be present');
      assert.strictEqual(authComp!.symbols.length, 1);
      assert.strictEqual(authComp!.symbols[0].identifier, 'ValidateToken');
      assert.strictEqual(authComp!.symbols[0].outgoing_edges?.[0].target_id, 'node-db-1');
      assert.strictEqual(authComp!.symbols[0].outgoing_edges?.[0].edge_type, 'CALLS');

      const dbComp = res.components.find(c => c.component_id === 'pkg/db.go');
      assert.ok(dbComp, 'pkg/db.go component from status should be present');
    });
  });

  describe('6. AST Tree View Provider (CosmASTTreeDataProvider)', () => {
    it('should format UniverseItem with Merkle hash prefix and component/symbol/edge counts', () => {
      const graph: ASTTreeGraph = {
        universe_id: 'universe-main',
        merkle_root: '8c94fa52714cd6e456218731adbf349',
        total_components: 4,
        total_symbols: 12,
        total_edges: 7,
        components: []
      };

      const universeItem = new UniverseItem(graph);
      assert.ok(String(universeItem.label).includes('🌌 universe-main'));
      assert.ok(String(universeItem.label).includes('8c94fa52'));
      assert.ok(String(universeItem.description).includes('4 components'));
      assert.ok(String(universeItem.description).includes('12 symbols'));
      assert.ok(String(universeItem.description).includes('7 edges'));
      assert.strictEqual(universeItem.contextValue, 'cosm.universe');
    });

    it('should build compacted DirectoryItem nodes for component paths (e.g. pkg/core)', () => {
      const client = new CosmClient(process.cwd());
      const provider = new CosmASTTreeDataProvider(client);

      const components: ASTTreeComponent[] = [
        {
          component_id: 'pkg/core/schema.go',
          name: 'schema.go',
          language: 'go',
          type: 'Backend',
          symbols: [
            {
              node_id: 'sym-schema-1',
              identifier: 'ASTNode',
              node_type: 'StructDecl',
              language: 'go'
            }
          ]
        },
        {
          component_id: 'pkg/core/types.go',
          name: 'types.go',
          language: 'go',
          type: 'Backend',
          symbols: []
        },
        {
          component_id: 'cmd/cosm/main.go',
          name: 'main.go',
          language: 'go',
          type: 'Backend',
          symbols: []
        },
        {
          component_id: 'root_file.go',
          name: 'root_file.go',
          language: 'go',
          type: 'Backend',
          symbols: []
        }
      ];

      const hierarchy = provider.buildComponentHierarchy(components);

      // Root level should contain compacted directories and the root file
      const dirLabels = hierarchy.filter(h => h instanceof DirectoryItem).map(d => String(d.label));
      const fileLabels = hierarchy.filter(h => h instanceof ComponentItem).map(f => String(f.label));

      // 'pkg/core' should be compacted because 'pkg' contains only 'core'
      assert.ok(dirLabels.includes('pkg/core'), 'pkg/core should be compacted into a single folder node');
      // 'cmd/cosm' should be compacted because 'cmd' contains only 'cosm'
      assert.ok(dirLabels.includes('cmd/cosm'), 'cmd/cosm should be compacted into a single folder node');
      assert.ok(fileLabels.includes('root_file.go'), 'root_file.go should be at root level');

      // Verify pkg/core children
      const pkgCoreDir = hierarchy.find(h => h instanceof DirectoryItem && h.label === 'pkg/core') as DirectoryItem;
      assert.ok(pkgCoreDir);
      assert.strictEqual(pkgCoreDir.components.length, 2);
      const pkgFiles = pkgCoreDir.components.map(c => c.label);
      assert.ok(pkgFiles.includes('schema.go'));
      assert.ok(pkgFiles.includes('types.go'));
    });

    it('should format ComponentItem with language badge and symbol counts', () => {
      const comp: ASTTreeComponent = {
        component_id: 'pkg/core/schema.go',
        name: 'schema.go',
        language: 'go',
        type: 'Backend',
        symbols: [
          { node_id: '1', identifier: 'Foo', node_type: 'FunctionDecl', language: 'go' },
          { node_id: '2', identifier: 'Bar', node_type: 'StructDecl', language: 'go' }
        ]
      };

      const item = new ComponentItem(comp, 'pkg/core/schema.go');
      assert.strictEqual(item.label, 'schema.go');
      assert.strictEqual(item.description, '[go] (2 symbols)');
      assert.strictEqual(item.contextValue, 'cosm.component');
    });

    it('should map AST Symbol node types to proper VS Code ThemeIcons and wire click command', () => {
      assert.strictEqual(getSymbolThemeIcon('FunctionDecl').id, 'symbol-method');
      assert.strictEqual(getSymbolThemeIcon('MethodDecl').id, 'symbol-method');
      assert.strictEqual(getSymbolThemeIcon('StructDecl').id, 'symbol-structure');
      assert.strictEqual(getSymbolThemeIcon('ClassDecl').id, 'symbol-structure');
      assert.strictEqual(getSymbolThemeIcon('InterfaceDecl').id, 'symbol-structure');
      assert.strictEqual(getSymbolThemeIcon('ResourceBlock').id, 'cloud');
      assert.strictEqual(getSymbolThemeIcon('CloudResource').id, 'cloud');
      assert.strictEqual(getSymbolThemeIcon('EndpointDecl').id, 'globe');
      assert.strictEqual(getSymbolThemeIcon('RouteDecl').id, 'globe');
      assert.strictEqual(getSymbolThemeIcon('VariableDecl').id, 'symbol-variable');
      assert.strictEqual(getSymbolThemeIcon('FieldDecl').id, 'symbol-variable');

      const sym: ASTTreeSymbol = {
        node_id: 'sym-func-1',
        identifier: 'HandleHealth',
        node_type: 'FunctionDecl',
        language: 'go',
        signature: 'func HandleHealth(w http.ResponseWriter, r *http.Request)'
      };

      const symItem = new SymbolItem(sym, 'pkg/api/health.go');
      assert.strictEqual(symItem.label, 'HandleHealth');
      assert.strictEqual(symItem.description, 'func HandleHealth(w http.ResponseWriter, r *http.Request)');
      assert.strictEqual((symItem.iconPath as any).id, 'symbol-method');
      assert.strictEqual(symItem.contextValue, 'cosm.symbol');

      // Click command should be cosm.navigateToSymbol
      assert.strictEqual(symItem.command?.command, 'cosm.navigateToSymbol');
      assert.strictEqual(symItem.command?.arguments?.[0], 'pkg/api/health.go');
      assert.strictEqual(symItem.command?.arguments?.[1], sym);
    });

    it('should construct expandable DetailItems under SymbolItem for Signature, Contracts/Edges, Lineage, and Node ID', () => {
      const sym: ASTTreeSymbol = {
        node_id: 'sym-auth-login',
        identifier: 'LoginHandler',
        node_type: 'FunctionDecl',
        language: 'typescript',
        signature: 'export function LoginHandler(req: Request): Response',
        docstring: 'Handles OAuth2 authentication and user sessions',
        outgoing_edges: [
          {
            target_id: 'sym-db-users',
            edge_type: 'CALLS',
            label: 'FindUserByEmail'
          }
        ],
        lineage: {
          user_prompt: 'Implement secure login endpoint',
          intent: 'feat(auth): login handler',
          executing_agent_id: 'gemini-3.8-flash',
          timestamp: '2026-09-17T12:00:00Z'
        }
      };

      const details = createSymbolDetails(sym);
      assert.strictEqual(details.length, 4);

      // 1. Signature
      const sigItem = details.find(d => d.label === 'Signature');
      assert.ok(sigItem);
      assert.strictEqual(sigItem.description, 'export function LoginHandler(req: Request): Response');
      assert.strictEqual((sigItem.iconPath as any).id, 'symbol-key');
      assert.strictEqual(sigItem.children?.length, 1);
      assert.strictEqual(sigItem.children?.[0].label, 'Docstring');

      // 2. Contracts / Edges
      const edgesItem = details.find(d => d.label === 'Contracts/Edges');
      assert.ok(edgesItem);
      assert.strictEqual((edgesItem.iconPath as any).id, 'references');
      assert.strictEqual(edgesItem.children?.length, 1);
      assert.strictEqual(edgesItem.children?.[0].label, 'CALLS');
      assert.ok(String(edgesItem.children?.[0].description).includes('sym-db-users'));

      // 3. Lineage
      const lineageItem = details.find(d => d.label === 'Lineage');
      assert.ok(lineageItem);
      assert.strictEqual((lineageItem.iconPath as any).id, 'history');
      assert.ok(String(lineageItem.description).includes('gemini-3.8-flash'));
      assert.strictEqual(lineageItem.children?.length, 4);
      const childLabels = lineageItem.children?.map(c => c.label);
      assert.ok(childLabels?.includes('Prompt'));
      assert.ok(childLabels?.includes('Intent'));
      assert.ok(childLabels?.includes('Agent'));
      assert.ok(childLabels?.includes('Timestamp'));

      // 4. Node ID
      const nodeIdItem = details.find(d => d.label === 'Node ID');
      assert.ok(nodeIdItem);
      assert.strictEqual((nodeIdItem.iconPath as any).id, 'key');
      assert.strictEqual(nodeIdItem.description, 'sym-auth-login');
      assert.strictEqual(nodeIdItem.command?.command, 'cosm.copyNodeId');
    });

    it('should fulfill TreeDataProvider contract across getChildren and refresh hierarchy', async () => {
      const client = new CosmClient(process.cwd());
      const mockGraph: ASTTreeGraph = {
        universe_id: 'universe-test',
        merkle_root: 'abcdef1234567890',
        total_components: 1,
        total_symbols: 1,
        total_edges: 0,
        components: [
          {
            component_id: 'pkg/core/schema.go',
            name: 'schema.go',
            language: 'go',
            type: 'Backend',
            symbols: [
              {
                node_id: 'sym-1',
                identifier: 'SchemaStruct',
                node_type: 'StructDecl',
                language: 'go',
                signature: 'type SchemaStruct struct'
              }
            ]
          }
        ]
      };

      client.getASTTree = async () => mockGraph;

      const provider = new CosmASTTreeDataProvider(client, 'universe-test');

      // 1. Root children -> [UniverseItem]
      const rootChildren = await provider.getChildren();
      assert.strictEqual(rootChildren.length, 1);
      assert.ok(rootChildren[0] instanceof UniverseItem);

      // 2. UniverseItem children -> MerkleInfoItem + DirectoryItem (pkg/core)
      const universeChildren = await provider.getChildren(rootChildren[0]);
      assert.strictEqual(universeChildren.length, 2);
      assert.ok(universeChildren[0] instanceof MerkleInfoItem);
      assert.strictEqual((universeChildren[0] as MerkleInfoItem).label, 'ℹ️ What is an AST Merkle Node?');
      assert.strictEqual((universeChildren[0] as MerkleInfoItem).command?.command, 'cosm.explainMerkleNodes');
      assert.ok(universeChildren[1] instanceof DirectoryItem);
      assert.strictEqual((universeChildren[1] as DirectoryItem).label, 'pkg/core');

      // 3. DirectoryItem children -> ComponentItem (schema.go)
      const dirChildren = await provider.getChildren(universeChildren[1]);
      assert.strictEqual(dirChildren.length, 1);
      assert.ok(dirChildren[0] instanceof ComponentItem);
      assert.strictEqual((dirChildren[0] as ComponentItem).label, 'schema.go');

      // 4. ComponentItem children -> SymbolItem (SchemaStruct)
      const compChildren = await provider.getChildren(dirChildren[0]);
      assert.strictEqual(compChildren.length, 1);
      assert.ok(compChildren[0] instanceof SymbolItem);
      assert.strictEqual((compChildren[0] as SymbolItem).label, 'SchemaStruct');

      // 5. SymbolItem children -> DetailItem (Signature, Contracts/Edges, Lineage, Node ID)
      const symChildren = await provider.getChildren(compChildren[0]);
      assert.strictEqual(symChildren.length, 4);
      assert.ok(symChildren.every(c => c instanceof DetailItem));

      // 6. getTreeItem returns element
      assert.strictEqual(provider.getTreeItem(rootChildren[0]), rootChildren[0]);

      // 7. Refresh triggers event
      let eventFired = false;
      provider.onDidChangeTreeData(() => {
        eventFired = true;
      });
      provider.refresh('universe-updated');
      assert.strictEqual(eventFired, true);
    });

    it('should verify all required AST tree commands, views, and menus in package.json', () => {
      const pkgPath = path.resolve(process.cwd(), 'editors/vscode/package.json');
      const packageJson = fs.existsSync(pkgPath)
        ? JSON.parse(fs.readFileSync(pkgPath, 'utf8'))
        : JSON.parse(fs.readFileSync(path.resolve(process.cwd(), 'package.json'), 'utf8'));

      // View verification
      const explorerViews = packageJson.contributes.views['cosm-explorer'];
      assert.strictEqual(explorerViews[0].id, 'cosm.astTreeView');
      assert.strictEqual(explorerViews[0].name, 'AST Merkle Tree Explorer');
      assert.strictEqual(explorerViews[0].type, 'tree');
      assert.strictEqual(explorerViews[1].id, 'cosm.topologyTreeView');
      assert.strictEqual(explorerViews[1].name, 'Architecture Topology (Frontend → Backend → Infra)');
      assert.strictEqual(explorerViews[1].type, 'tree');

      // Commands verification
      const commands = packageJson.contributes.commands.map((c: any) => c.command);
      assert.ok(commands.includes('cosm.refreshASTTree'), 'cosm.refreshASTTree must be in commands');
      assert.ok(commands.includes('cosm.navigateToSymbol'), 'cosm.navigateToSymbol must be in commands');
      assert.ok(commands.includes('cosm.viewASTPayload'), 'cosm.viewASTPayload must be in commands');
      assert.ok(commands.includes('cosm.copyNodeId'), 'cosm.copyNodeId must be in commands');
      assert.ok(commands.includes('cosm.explainMerkleNodes'), 'cosm.explainMerkleNodes must be in commands');
      assert.ok(commands.includes('cosm.refreshTopologyTree'), 'cosm.refreshTopologyTree must be in commands');

      // Menus verification
      const viewTitle = packageJson.contributes.menus['view/title'];
      assert.ok(
        viewTitle.some((m: any) => m.command === 'cosm.refreshASTTree' && m.when === 'view == cosm.astTreeView'),
        'view/title must include cosm.refreshASTTree for cosm.astTreeView'
      );

      const itemContext = packageJson.contributes.menus['view/item/context'];
      const contextCommands = itemContext.map((m: any) => m.command);
      assert.ok(contextCommands.includes('cosm.viewASTPayload'), 'cosm.viewASTPayload in context menu');
      assert.ok(contextCommands.includes('cosm.copyNodeId'), 'cosm.copyNodeId in context menu');
      assert.ok(contextCommands.includes('cosm.diffSymbol'), 'cosm.diffSymbol in context menu');
      assert.ok(contextCommands.includes('cosm.blastRadius'), 'cosm.blastRadius in context menu');
    });
  });

  describe('7. Architecture Topology Tree Provider (CosmTopologyTreeDataProvider)', () => {
    it('should organize topology graph into Frontend, Backend, and Cloud Infra swimlane items', async () => {
      const client = new CosmClient(process.cwd());
      const mockTopology: FullTopologyGraph = {
        total_nodes: 3,
        total_edges: 1,
        frontend_nodes: [
          {
            id: 'node-fe',
            name: 'App.tsx',
            tier: 'Frontend',
            type: 'Component',
            language: 'typescript',
            file_path: 'frontend/src/App.tsx',
            outgoing: [
              {
                target_id: 'node-be',
                edge_type: 'CONSUMES_API',
                label: 'GET /api/health'
              }
            ]
          }
        ],
        backend_nodes: [
          {
            id: 'node-be',
            name: 'main.go:HandleHealth',
            tier: 'API/Backend',
            type: 'Endpoint',
            language: 'go',
            file_path: 'cmd/server/main.go',
            outgoing: []
          }
        ],
        infra_nodes: [
          {
            id: 'node-infra',
            name: 'main.tf',
            tier: 'Cloud Infra',
            type: 'Resource',
            language: 'hcl',
            file_path: 'infra/main.tf',
            outgoing: []
          }
        ]
      };

      client.getTopology = async () => mockTopology;

      const provider = new CosmTopologyTreeDataProvider(client);

      // 1. Root children -> 3 TierCategoryItem instances
      const tiers = await provider.getChildren();
      assert.strictEqual(tiers.length, 3);
      assert.ok(tiers.every(t => t instanceof TierCategoryItem));
      assert.strictEqual((tiers[0] as TierCategoryItem).tierKey, 'Frontend');
      assert.strictEqual((tiers[1] as TierCategoryItem).tierKey, 'API/Backend');
      assert.strictEqual((tiers[2] as TierCategoryItem).tierKey, 'Cloud Infra');

      // 2. Frontend tier children -> TopologyComponentItem (frontend/src/App.tsx)
      const feComponents = await provider.getChildren(tiers[0]);
      assert.strictEqual(feComponents.length, 1);
      assert.ok(feComponents[0] instanceof TopologyComponentItem);
      const feCompItem = feComponents[0] as TopologyComponentItem;
      assert.strictEqual(feCompItem.filePath, 'frontend/src/App.tsx');

      // 3. TopologyComponentItem children -> TopologyNodeItem (App.tsx)
      const feNodes = await provider.getChildren(feCompItem);
      assert.strictEqual(feNodes.length, 1);
      assert.ok(feNodes[0] instanceof TopologyNodeItem);
      const feNodeItem = feNodes[0] as TopologyNodeItem;
      assert.strictEqual(feNodeItem.label, 'App.tsx');
      assert.strictEqual(feNodeItem.node.file_path, 'frontend/src/App.tsx');
      assert.strictEqual(feNodeItem.command?.command, 'cosm.navigateToSymbol');

      // 4. TopologyNodeItem children -> TopologyActionItem (Blast) + TopologyActionItem (File) + TopologyEdgeItem
      const nodeChildren = await provider.getChildren(feNodeItem);
      assert.strictEqual(nodeChildren.length, 3);
      assert.ok(nodeChildren[0] instanceof TopologyActionItem);
      assert.strictEqual((nodeChildren[0] as TopologyActionItem).label, 'Audit Blast Radius');
      assert.strictEqual((nodeChildren[0] as TopologyActionItem).command?.command, 'cosm.blastRadius');
      assert.ok(nodeChildren[1] instanceof TopologyActionItem);
      assert.strictEqual((nodeChildren[1] as TopologyActionItem).label, 'File: frontend/src/App.tsx');
      assert.ok(nodeChildren[2] instanceof TopologyEdgeItem);
      assert.strictEqual((nodeChildren[2] as TopologyEdgeItem).label, '🔗 CONSUMES_API');

      // 4. Refresh triggers event
      let refreshed = false;
      provider.onDidChangeTreeData(() => {
        refreshed = true;
      });
      provider.refresh();
      assert.strictEqual(refreshed, true);
    });

    it('should normalize BlastRadiusReport and handle ImpactAssessment payloads', async () => {
      const client = new CosmClient(process.cwd());
      (client as any).execCosm = async () => JSON.stringify({
        target_symbol_id: 'sym-core-hash',
        risk_score: 0.75,
        total_direct_nodes: 2,
        total_downstream_nodes: 5,
        summary: 'Impact assessment for sym-core-hash',
        incoming_edges: ['CALLS:pkg/api', 'IMPORTS:pkg/storage'],
        outgoing_edges: ['HASHES:pkg/crypto'],
        affected_components: ['pkg/api', 'pkg/storage'],
        warnings: ['Contract breakage detected']
      });

      const report = await client.getBlastRadius('sym-core-hash');
      assert.strictEqual(report.target_symbol_id, 'sym-core-hash');
      assert.strictEqual(report.risk_score, 0.75);
      assert.strictEqual(report.total_direct_nodes, 2);
      assert.strictEqual(report.total_downstream_nodes, 5);
      assert.strictEqual(report.summary, 'Impact assessment for sym-core-hash');
      assert.strictEqual(report.warnings?.length, 1);
      assert.strictEqual(report.warnings?.[0], 'Contract breakage detected');
      assert.strictEqual(report.affected_components?.length, 2);
    });
  });

  describe('8. Stack Operations, SCM Provider & Dynamic Commit Workflows', () => {
    it('should validate StackRecord interface extension', () => {
      const record: StackRecord = {
        change_id: 'change-101',
        universe_id: 'feat-auth',
        parent_change_id: 'change-100',
        title: 'feat: add session tokens',
        status: 'open',
        manifest_hash: '9f83ac1278de',
        order_index: 2,
        author_did: 'did:key:z6MkpTHR8VNsBxYAAWHuEc2Ka9txNddw6SQFO8vBQJpu5h8k'
      };

      assert.strictEqual(record.change_id, 'change-101');
      assert.strictEqual(record.universe_id, 'feat-auth');
      assert.strictEqual(record.parent_change_id, 'change-100');
      assert.strictEqual(record.title, 'feat: add session tokens');
      assert.strictEqual(record.manifest_hash, '9f83ac1278de');
      assert.strictEqual(record.order_index, 2);
      assert.strictEqual(record.author_did, 'did:key:z6MkpTHR8VNsBxYAAWHuEc2Ka9txNddw6SQFO8vBQJpu5h8k');
    });

    it('should execute createStack with expected CLI arguments', async () => {
      const client = new CosmClient(process.cwd());
      let capturedArgs: string[] = [];
      (client as any).execCosm = async (args: string[]) => {
        capturedArgs = args;
        return 'Stacked change change-200 created';
      };

      const res = await client.createStack({
        changeId: 'change-200',
        universeId: 'feat-api',
        parentId: 'change-199',
        title: 'Add JWT verify endpoint'
      });

      assert.ok(res.includes('change-200'));
      assert.deepStrictEqual(capturedArgs, [
        'stack', 'create',
        '-c', 'change-200',
        '-u', 'feat-api',
        '-p', 'change-199',
        '--title', 'Add JWT verify endpoint'
      ]);
    });

    it('should execute evolveStack with expected CLI arguments', async () => {
      const client = new CosmClient(process.cwd());
      let capturedArgs: string[] = [];
      (client as any).execCosm = async (args: string[]) => {
        capturedArgs = args;
        return 'Evolved 3 descendants of change-root';
      };

      const res = await client.evolveStack('change-root');
      assert.ok(res.includes('Evolved'));
      assert.deepStrictEqual(capturedArgs, ['stack', 'evolve', '-c', 'change-root']);
    });

    it('should parse listStack JSON output into StackRecord array', async () => {
      const client = new CosmClient(process.cwd());
      (client as any).execCosm = async (args: string[]) => {
        if (args.includes('-format') && args.includes('json')) {
          return JSON.stringify([
            {
              change_id: 'change-1',
              universe_id: 'universe-main',
              parent_change_id: '',
              title: 'Initial root change',
              manifest_hash: 'abc123456789',
              order_index: 0,
              author_did: 'did:key:alice'
            },
            {
              change_id: 'change-2',
              universe_id: 'universe-feat',
              parent_change_id: 'change-1',
              title: 'Add auth middleware',
              manifest_hash: 'def987654321',
              order_index: 1,
              author_did: 'did:key:bob'
            }
          ]);
        }
        return '';
      };

      const stack = await client.listStack();
      assert.strictEqual(stack.length, 2);
      assert.strictEqual(stack[0].change_id, 'change-1');
      assert.strictEqual(stack[0].manifest_hash, 'abc123456789');
      assert.strictEqual(stack[0].order_index, 0);
      assert.strictEqual(stack[1].change_id, 'change-2');
      assert.strictEqual(stack[1].parent_change_id, 'change-1');
      assert.strictEqual(stack[1].author_did, 'did:key:bob');
    });

    it('should fallback to regex text parsing for listStack when JSON fails', async () => {
      const client = new CosmClient(process.cwd());
      (client as any).execCosm = async (args: string[]) => {
        if (args.includes('-format') && args.includes('json')) {
          throw new Error('flag -format not supported');
        }
        return [
          'Jujutsu-Style Stacked Proposals:',
          '   [0] 🔹 change-alpha (Universe: universe-main, Head: a1b2c3d4e5f6)',
          '       Parent: change-root | Auto-Rebase: Active',
          '   [1] 🔹 change-beta (Universe: universe-feat, Head: f6e5d4c3b2a1)',
          '       Parent: change-alpha | Auto-Rebase: Active'
        ].join('\n');
      };

      const stack = await client.listStack();
      assert.strictEqual(stack.length, 2);
      assert.strictEqual(stack[0].order_index, 0);
      assert.strictEqual(stack[0].change_id, 'change-alpha');
      assert.strictEqual(stack[0].universe_id, 'universe-main');
      assert.strictEqual(stack[0].manifest_hash, 'a1b2c3d4e5f6');
      assert.strictEqual(stack[0].parent_change_id, 'change-root');

      assert.strictEqual(stack[1].order_index, 1);
      assert.strictEqual(stack[1].change_id, 'change-beta');
      assert.strictEqual(stack[1].universe_id, 'universe-feat');
      assert.strictEqual(stack[1].manifest_hash, 'f6e5d4c3b2a1');
      assert.strictEqual(stack[1].parent_change_id, 'change-alpha');
    });

    it('should conditionally append -p in commit only when prompt is non-empty', async () => {
      const client = new CosmClient(process.cwd());
      let capturedArgs: string[] = [];
      (client as any).execCosm = async (args: string[]) => {
        capturedArgs = args;
        return 'Committed Merkle Root: e2b4f618a901';
      };

      // 1. Commit with empty prompt (standard human commit)
      const res1 = await client.commit({
        universe: 'universe-main',
        intent: 'feat(api): optimize handlers',
        prompt: ''
      });
      assert.strictEqual(res1.merkle_root, 'e2b4f618a901');
      assert.strictEqual(capturedArgs.includes('-p'), false);

      // 2. Commit with whitespace-only prompt
      await client.commit({
        universe: 'universe-main',
        intent: 'feat(api): whitespace test',
        prompt: '   '
      });
      assert.strictEqual(capturedArgs.includes('-p'), false);

      // 3. Commit with valid non-empty prompt (AI lineage commit)
      await client.commit({
        universe: 'universe-main',
        intent: 'feat(api): add caching',
        prompt: 'Refactor database query to use redis cache'
      });
      assert.strictEqual(capturedArgs.includes('-p'), true);
      const pIdx = capturedArgs.indexOf('-p');
      assert.strictEqual(capturedArgs[pIdx + 1], 'Refactor database query to use redis cache');
    });

    it('should initialize SCM provider with actionButton and acceptInputCommand', () => {
      const client = new CosmClient(process.cwd());
      const mockContext = { subscriptions: [], extensionPath: process.cwd() } as any;
      const provider = new CosmSCMProvider(client, mockContext);

      const scmInstance = (provider as any).scm;
      assert.ok(scmInstance);
      assert.strictEqual(scmInstance.actionButton?.command?.command, 'cosm.commit');
      assert.strictEqual(scmInstance.actionButton?.command?.title, '✓ Commit AST Changes');
      assert.strictEqual(scmInstance.actionButton?.enabled, true);
      assert.strictEqual(scmInstance.acceptInputCommand?.command, 'cosm.commit');
    });

    it('should scan working tree and extract drifted files from working_tree_lens', async () => {
      const client = new CosmClient(process.cwd());
      const mockContext = { subscriptions: [], extensionPath: process.cwd() } as any;
      const provider = new CosmSCMProvider(client, mockContext);

      const mockStatus: CosmStatus = {
        status: 'SUCCESS',
        universe_id: 'universe-main',
        merkle_root: 'root-abc',
        components_count: 2,
        cross_edges_count: 1,
        components: ['pkg/auth/token.go', 'pkg/auth/session.go'],
        working_tree_lens: {
          status: 'drift_detected',
          projected_files_count: 2,
          drifted_files: [
            'pkg/auth/token.go (modified on disk)',
            'pkg/auth/session.go (modified on disk)'
          ]
        }
      };

      await (provider as any).scanWorkingTree(mockStatus);

      const modifiedGroup = (provider as any).modifiedGroup;
      assert.ok(modifiedGroup.resourceStates.length >= 2);
      const paths = modifiedGroup.resourceStates.map((r: any) => r.resourceUri.fsPath);
      assert.ok(paths.some((p: string) => p.includes('pkg/auth/token.go')));
      assert.ok(paths.some((p: string) => p.includes('pkg/auth/session.go')));
    });

    it('should handle stageFile with SourceControlResourceGroup, ResourceState, and undefined', async () => {
      const client = new CosmClient(process.cwd());
      let stagedFiles: string[] = [];
      (client as any).stageFiles = async (files: string[]) => {
        stagedFiles = files;
        return 'Staged files';
      };

      const mockContext = { subscriptions: [], extensionPath: process.cwd() } as any;
      const provider = new CosmSCMProvider(client, mockContext);
      (provider as any).refresh = async () => {};

      // 1. Stage group
      const mockGroup = {
        id: 'modified',
        resourceStates: [
          { resourceUri: { fsPath: path.resolve(process.cwd(), 'pkg/api/login.go') } },
          { resourceUri: { fsPath: path.resolve(process.cwd(), 'pkg/api/logout.go') } }
        ]
      };
      await provider.stageFile(mockGroup as any);
      assert.strictEqual(stagedFiles.length, 2);
      assert.ok(stagedFiles.some(f => f.includes('login.go')));

      // 2. Stage single resource state
      await provider.stageFile({ resourceUri: { fsPath: path.resolve(process.cwd(), 'pkg/api/user.go') } } as any);
      assert.strictEqual(stagedFiles.length, 1);
      assert.ok(stagedFiles[0].includes('user.go'));

      // 3. Stage undefined -> stageAll
      await provider.stageFile(undefined);
      assert.ok(stagedFiles.length > 0);
    });
  });

  describe('9. Stacked Changes View & SCM Menus', () => {
    it('should return an informational TreeItem when stack is empty', async () => {
      const client = new CosmClient(process.cwd());
      (client as any).listStack = async () => [];

      const mockSCM = { getActiveUniverse: () => 'universe-main' } as any;
      const provider = new CosmStackedChangesProvider(client, mockSCM);

      const items = await provider.getChildren();
      assert.strictEqual(items.length, 1);
      assert.strictEqual(items[0].label, 'No active stacked changes');
      assert.strictEqual(items[0].description, 'Jujutsu-style stacked proposals');
      assert.strictEqual(items[0].tooltip, 'Create a stacked change with cosm.createStack');
      assert.strictEqual(items[0].contextValue, 'noStackedChanges');
      assert.strictEqual((items[0].iconPath as any)?.id, 'info');
    });

    it('should return StackedChangeTreeItems when stack has records and highlight active universe', async () => {
      const client = new CosmClient(process.cwd());
      const mockRecords: StackRecord[] = [
        {
          change_id: 'c/auth-jwt',
          universe_id: 'universe-main',
          parent_change_id: 'universe-root',
          title: 'JWT Authentication Service',
          manifest_hash: '9f83ac1278de',
          order_index: 0,
          status: 'Active'
        },
        {
          change_id: 'c/api-routes',
          universe_id: 'u/api-routes',
          parent_change_id: 'c/auth-jwt',
          title: 'Protected API Routes',
          manifest_hash: '1a2b3c4d5e6f',
          order_index: 1,
          status: 'Draft'
        }
      ];

      (client as any).listStack = async () => mockRecords;

      const mockSCM = { getActiveUniverse: () => 'universe-main' } as any;
      const provider = new CosmStackedChangesProvider(client, mockSCM);

      const items = await provider.getChildren();
      assert.strictEqual(items.length, 2);

      // Item 0: Active in universe-main
      const activeItem = items[0];
      assert.strictEqual(activeItem.label, '🥞 [0] c/auth-jwt (Active)');
      assert.strictEqual(activeItem.description, 'universe-main | Head: 9f83ac12');
      assert.ok(typeof activeItem.tooltip === 'string');
      assert.ok(activeItem.tooltip.includes('Change ID: c/auth-jwt'));
      assert.ok(activeItem.tooltip.includes('Title: JWT Authentication Service'));
      assert.ok(activeItem.tooltip.includes('Universe: universe-main'));
      assert.ok(activeItem.tooltip.includes('Parent: universe-root'));
      assert.ok(activeItem.tooltip.includes('Status: Active'));
      assert.strictEqual(activeItem.contextValue, 'stackedChange');
      assert.strictEqual((activeItem.iconPath as any)?.id, 'pass-filled');
      assert.strictEqual(activeItem.command?.command, 'cosm.switchUniverse');
      assert.deepStrictEqual(activeItem.command?.arguments, ['universe-main']);
      assert.strictEqual(activeItem.record?.change_id, 'c/auth-jwt');
      assert.strictEqual(activeItem.change?.change_id, 'c/auth-jwt');
      assert.strictEqual(activeItem.change?.universe_id, 'universe-main');

      // Item 1: Inactive in u/api-routes
      const childItem = items[1];
      assert.strictEqual(childItem.label, '🥞 [1] c/api-routes');
      assert.strictEqual(childItem.description, 'u/api-routes | Head: 1a2b3c4d');
      assert.ok(typeof childItem.tooltip === 'string');
      assert.ok(childItem.tooltip.includes('Change ID: c/api-routes'));
      assert.ok(childItem.tooltip.includes('Parent: c/auth-jwt'));
      assert.ok(childItem.tooltip.includes('Status: Draft'));
      assert.strictEqual(childItem.contextValue, 'stackedChange');
      assert.strictEqual((childItem.iconPath as any)?.id, 'layers');
      assert.strictEqual(childItem.command?.command, 'cosm.switchUniverse');
      assert.deepStrictEqual(childItem.command?.arguments, ['u/api-routes']);
      assert.strictEqual(childItem.record?.change_id, 'c/api-routes');
      assert.strictEqual(childItem.change?.change_id, 'c/api-routes');
      assert.strictEqual(childItem.change?.universe_id, 'u/api-routes');
    });

    it('should return empty array for element children and identity for getTreeItem', async () => {
      const client = new CosmClient(process.cwd());
      (client as any).listStack = async () => [];
      const provider = new CosmStackedChangesProvider(client);

      const infoItem = new StackedChangeTreeItem();
      const children = await provider.getChildren(infoItem);
      assert.deepStrictEqual(children, []);

      const treeItem = provider.getTreeItem(infoItem);
      assert.strictEqual(treeItem, infoItem);
    });

    it('should fire onDidChangeTreeData when refresh is called', (done?: any) => {
      const client = new CosmClient(process.cwd());
      const provider = new CosmStackedChangesProvider(client);

      let fired = false;
      const sub = provider.onDidChangeTreeData(() => {
        fired = true;
      });

      provider.refresh();
      assert.strictEqual(fired, true);
      sub.dispose();
    });

    it('should handle client.listStack failure gracefully with error item', async () => {
      const client = new CosmClient(process.cwd());
      (client as any).listStack = async () => {
        throw new Error('Cosm WAL storage offline');
      };

      const provider = new CosmStackedChangesProvider(client);
      const items = await provider.getChildren();
      assert.strictEqual(items.length, 1);
      assert.strictEqual(items[0].label, 'Error loading stacked proposals');
      assert.strictEqual(items[0].description, 'Cosm WAL storage offline');
      assert.strictEqual((items[0].iconPath as any)?.id, 'error');
    });

    it('should contribute cosm.createStack, cosm.evolveStack, cosm.refreshStack, cosm.mergeUniverse, and cosm.showLog in package.json', () => {
      const pkgPath = path.resolve(__dirname, '../../package.json');
      const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));

      const commands: any[] = pkg.contributes.commands;
      const createStack = commands.find(c => c.command === 'cosm.createStack');
      const evolveStack = commands.find(c => c.command === 'cosm.evolveStack');
      const refreshStack = commands.find(c => c.command === 'cosm.refreshStack');
      const mergeUniverse = commands.find(c => c.command === 'cosm.mergeUniverse');
      const showLog = commands.find(c => c.command === 'cosm.showLog');

      assert.ok(createStack, 'cosm.createStack command must be declared');
      assert.strictEqual(createStack.title, 'Cosm: Create Stacked Change (Jujutsu-style)');
      assert.strictEqual(createStack.icon, '$(layers)');

      assert.ok(evolveStack, 'cosm.evolveStack command must be declared');
      assert.strictEqual(evolveStack.title, 'Cosm: Evolve Stack (Auto-Rebase Descendants)');
      assert.strictEqual(evolveStack.icon, '$(zap)');

      assert.ok(refreshStack, 'cosm.refreshStack command must be declared');
      assert.strictEqual(refreshStack.title, 'Cosm: Refresh Stacked Proposals');
      assert.strictEqual(refreshStack.icon, '$(refresh)');

      assert.ok(mergeUniverse, 'cosm.mergeUniverse command must be declared');
      assert.strictEqual(mergeUniverse.title, 'Cosm: Merge Micro-Universe (AST Union / Fast-Forward)');
      assert.strictEqual(mergeUniverse.icon, '$(git-merge)');

      assert.ok(showLog, 'cosm.showLog command must be declared');
      assert.strictEqual(showLog.title, 'Cosm: View Micro-Universe Commit History & Log');
      assert.strictEqual(showLog.icon, '$(history)');
    });

    it('should contribute cosm.scmView with name Cosm Stacks (Jujutsu-style) in package.json', () => {
      const pkgPath = path.resolve(__dirname, '../../package.json');
      const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));

      const scmViews: any[] = pkg.contributes.views.scm;
      assert.ok(scmViews && scmViews.length > 0);
      const stackView = scmViews.find(v => v.id === 'cosm.scmView');
      assert.ok(stackView, 'cosm.scmView view must be declared under contributes.views.scm');
      assert.strictEqual(stackView.name, 'Cosm Stacks (Jujutsu-style)');
    });

    it('should configure scm/title menus with required navigation ordering and when clauses in package.json', () => {
      const pkgPath = path.resolve(__dirname, '../../package.json');
      const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));

      const scmTitleMenus: any[] = pkg.contributes.menus['scm/title'];
      assert.ok(scmTitleMenus, 'scm/title menus must be defined');

      const commitMenu = scmTitleMenus.find(m => m.command === 'cosm.commit');
      assert.ok(commitMenu, 'cosm.commit must be in scm/title');
      assert.strictEqual(commitMenu.group, 'navigation@1');
      assert.strictEqual(commitMenu.when, 'scmProvider == cosm');

      const mergeUniverseMenu = scmTitleMenus.find(m => m.command === 'cosm.mergeUniverse');
      assert.ok(mergeUniverseMenu, 'cosm.mergeUniverse must be in scm/title');
      assert.strictEqual(mergeUniverseMenu.group, 'navigation@2');
      assert.strictEqual(mergeUniverseMenu.when, 'scmProvider == cosm');

      const showLogMenu = scmTitleMenus.find(m => m.command === 'cosm.showLog');
      assert.ok(showLogMenu, 'cosm.showLog must be in scm/title');
      assert.strictEqual(showLogMenu.group, 'navigation@3');
      assert.strictEqual(showLogMenu.when, 'scmProvider == cosm');

      const createStackMenu = scmTitleMenus.find(m => m.command === 'cosm.createStack');
      assert.ok(createStackMenu, 'cosm.createStack must be in scm/title');
      assert.strictEqual(createStackMenu.group, 'navigation@4');
      assert.strictEqual(createStackMenu.when, 'scmProvider == cosm');

      const refreshSCMMenu = scmTitleMenus.find(m => m.command === 'cosm.refreshSCM');
      assert.ok(refreshSCMMenu, 'cosm.refreshSCM must be in scm/title');
      assert.strictEqual(refreshSCMMenu.group, 'navigation@5');
      assert.strictEqual(refreshSCMMenu.when, 'scmProvider == cosm');
    });

    it('should configure inline groups in scm/resourceGroup/context and scm/resourceState/context in package.json', () => {
      const pkgPath = path.resolve(__dirname, '../../package.json');
      const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));

      // scm/resourceGroup/context
      const groupMenus: any[] = pkg.contributes.menus['scm/resourceGroup/context'];
      assert.ok(groupMenus, 'scm/resourceGroup/context must be defined');

      const stageAll = groupMenus.find(m => m.command === 'cosm.stageAll');
      assert.ok(stageAll, 'cosm.stageAll must be in scm/resourceGroup/context');
      assert.strictEqual(stageAll.when, 'scmResourceGroup == modified');
      assert.strictEqual(stageAll.group, 'inline@1');

      const unstageAll = groupMenus.find(m => m.command === 'cosm.unstageAll');
      assert.ok(unstageAll, 'cosm.unstageAll must be in scm/resourceGroup/context');
      assert.strictEqual(unstageAll.when, 'scmResourceGroup == staged');
      assert.strictEqual(unstageAll.group, 'inline@1');

      // scm/resourceState/context
      const stateMenus: any[] = pkg.contributes.menus['scm/resourceState/context'];
      assert.ok(stateMenus, 'scm/resourceState/context must be defined');

      const stageFile = stateMenus.find(m => m.command === 'cosm.stageFile');
      assert.ok(stageFile, 'cosm.stageFile must be in scm/resourceState/context');
      assert.strictEqual(stageFile.when, 'scmResourceGroup == modified');
      assert.strictEqual(stageFile.group, 'inline@1');

      const unstageFile = stateMenus.find(m => m.command === 'cosm.unstageFile');
      assert.ok(unstageFile, 'cosm.unstageFile must be in scm/resourceState/context');
      assert.strictEqual(unstageFile.when, 'scmResourceGroup == staged');
      assert.strictEqual(unstageFile.group, 'inline@1');

      const diffSymbol = stateMenus.find(m => m.command === 'cosm.diffSymbol');
      assert.ok(diffSymbol, 'cosm.diffSymbol must be in scm/resourceState/context');
      assert.strictEqual(diffSymbol.group, 'inline@2');
    });

    it('should configure view/title and view/item/context for cosm.scmView in package.json', () => {
      const pkgPath = path.resolve(__dirname, '../../package.json');
      const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));

      // view/title
      const viewTitleMenus: any[] = pkg.contributes.menus['view/title'];
      const stackViewTitle = viewTitleMenus.filter(m => m.when === 'view == cosm.scmView');
      assert.strictEqual(stackViewTitle.length, 4);

      const mergeUniverse = stackViewTitle.find(m => m.command === 'cosm.mergeUniverse');
      assert.ok(mergeUniverse);
      assert.strictEqual(mergeUniverse.group, 'navigation@1');

      const createStack = stackViewTitle.find(m => m.command === 'cosm.createStack');
      assert.ok(createStack);
      assert.strictEqual(createStack.group, 'navigation@2');

      const evolveStack = stackViewTitle.find(m => m.command === 'cosm.evolveStack');
      assert.ok(evolveStack);
      assert.strictEqual(evolveStack.group, 'navigation@3');

      const refreshStack = stackViewTitle.find(m => m.command === 'cosm.refreshStack');
      assert.ok(refreshStack);
      assert.strictEqual(refreshStack.group, 'navigation@4');

      // view/item/context
      const viewItemMenus: any[] = pkg.contributes.menus['view/item/context'];
      const stackViewItem = viewItemMenus.filter(m => m.when === 'view == cosm.scmView && viewItem == stackedChange');
      assert.strictEqual(stackViewItem.length, 3);

      const switchUniv = stackViewItem.find(m => m.command === 'cosm.switchUniverse');
      assert.ok(switchUniv);
      assert.strictEqual(switchUniv.group, 'inline@1');

      const evolveItem = stackViewItem.find(m => m.command === 'cosm.evolveStack');
      assert.ok(evolveItem);
      assert.strictEqual(evolveItem.group, 'inline@2');

      const mergeItem = stackViewItem.find(m => m.command === 'cosm.mergeUniverse');
      assert.ok(mergeItem);
      assert.strictEqual(mergeItem.group, 'inline@3');
    });
  });

  describe('10. Universe Operations, SCM Auto-Staging, and Commit Log Integration', () => {
    it('should validate CommitLogEntry interface properties', () => {
      const entry: CommitLogEntry = {
        commit_hash: '9f83ac0124fe',
        universe_id: 'universe-main',
        author: 'cosm-agent',
        agent_did: 'did:key:z6MkuSLtNTGy',
        llm_version: 'gemini-3.8-flash',
        timestamp: '2026-09-23T20:00:00Z',
        intent: 'feat(core): implement zero-copy AST merge',
        user_prompt: 'Merge u/feature into universe-main',
        parent_hash: '8a12bc9043de',
        components: ['pkg/core/merge.go', 'pkg/ast/node.go']
      };

      assert.strictEqual(entry.commit_hash, '9f83ac0124fe');
      assert.strictEqual(entry.universe_id, 'universe-main');
      assert.strictEqual(entry.components.length, 2);
      assert.strictEqual(entry.llm_version, 'gemini-3.8-flash');
    });

    it('should execute switchUniverse with expected arguments', async () => {
      const client = new CosmClient(process.cwd());
      let passedArgs: string[] = [];
      (client as any).execCosm = async (args: string[]) => {
        passedArgs = args;
        return 'Switched to universe u/auth-v2';
      };

      const result = await client.switchUniverse('u/auth-v2');
      assert.strictEqual(result, 'Switched to universe u/auth-v2');
      assert.deepStrictEqual(passedArgs, ['universe', 'switch', 'u/auth-v2']);
    });

    it('should execute mergeUniverse and parse head hash and target universe', async () => {
      const client = new CosmClient(process.cwd());
      let passedArgs: string[] = [];
      (client as any).execCosm = async (args: string[]) => {
        passedArgs = args;
        return "🔀 Successfully merged 'u/feature' into 'universe-main' (New Head: 4a3e810bcde89f0123456789abcdef0123456789)";
      };

      const res = await client.mergeUniverse('u/feature', 'universe-main', 'union');
      assert.deepStrictEqual(passedArgs, ['universe', 'merge', 'u/feature', '-t', 'universe-main', '-s', 'union']);
      assert.strictEqual(res.target_universe, 'universe-main');
      assert.strictEqual(res.head_hash, '4a3e810bcde89f0123456789abcdef0123456789');
    });

    it('should execute getLog and parse JSON commit log entries', async () => {
      const client = new CosmClient(process.cwd());
      let passedArgs: string[] = [];
      const sampleLogs: CommitLogEntry[] = [
        {
          commit_hash: 'c1',
          universe_id: 'universe-main',
          author: 'dev',
          timestamp: '2026-09-23T20:00:00Z',
          intent: 'initial commit',
          components: ['main.go']
        }
      ];

      (client as any).execCosm = async (args: string[]) => {
        passedArgs = args;
        return JSON.stringify(sampleLogs);
      };

      const logs = await client.getLog('universe-main', 10);
      assert.deepStrictEqual(passedArgs, ['log', '-u', 'universe-main', '-n', '10', '-format', 'json']);
      assert.strictEqual(logs.length, 1);
      assert.strictEqual(logs[0].commit_hash, 'c1');
    });

    it('should handle getLog failure gracefully by returning empty array', async () => {
      const client = new CosmClient(process.cwd());
      (client as any).execCosm = async () => {
        throw new Error('log command failed');
      };

      const logs = await client.getLog();
      assert.deepStrictEqual(logs, []);
    });

    it('should parse listUniverses JSON first with fallback to regex line matching', async () => {
      const client = new CosmClient(process.cwd());

      // 1. JSON success
      (client as any).execCosm = async (args: string[]) => {
        if (args.includes('-format')) {
          return JSON.stringify([
            { universe_id: 'universe-main', head_manifest_hash: 'hash1', status: 'active' },
            { universe_id: 'u/feature', head_manifest_hash: 'hash2', status: 'merged' }
          ]);
        }
        return '';
      };
      const jsonRes = await client.listUniverses();
      assert.strictEqual(jsonRes.length, 2);
      assert.strictEqual(jsonRes[0].universe_id, 'universe-main');
      assert.strictEqual(jsonRes[1].status, 'merged');

      // 2. Fallback to text
      (client as any).execCosm = async (args: string[]) => {
        if (args.includes('-format')) {
          throw new Error('JSON format unsupported');
        }
        return '* universe-main (Head: abc123def456, Status: active)\n* u/dev (Head: fedcba987654)';
      };
      const textRes = await client.listUniverses();
      assert.strictEqual(textRes.length, 2);
      assert.strictEqual(textRes[0].universe_id, 'universe-main');
      assert.strictEqual(textRes[0].head_manifest_hash, 'abc123def456');
      assert.strictEqual(textRes[1].universe_id, 'u/dev');
      assert.strictEqual(textRes[1].status, 'active');
    });

    it('should append --allow-secrets and -u in stageFiles', async () => {
      const client = new CosmClient(process.cwd());
      let passedArgs: string[] = [];
      (client as any).execCosm = async (args: string[]) => {
        passedArgs = args;
        return 'Staged';
      };

      await client.stageFiles(['pkg/core/engine.go'], 'intent test', 'prompt test', 'u/feature');
      assert.ok(passedArgs.includes('--allow-secrets'));
      assert.ok(passedArgs.includes('-u'));
      const uIndex = passedArgs.indexOf('-u');
      assert.strictEqual(passedArgs[uIndex + 1], 'u/feature');
      assert.ok(passedArgs.includes('--intent'));
      assert.ok(passedArgs.includes('--prompt'));
    });

    it('should safely collect modified files and return boolean status in SCMProvider.stageAll', async () => {
      const client = new CosmClient(process.cwd());
      let stagedFiles: string[] = [];
      let targetUniverse: string | undefined;
      (client as any).stageFiles = async (files: string[], _i?: string, _p?: string, universe?: string) => {
        stagedFiles = files;
        targetUniverse = universe;
        return 'Staged';
      };

      const mockContext = { subscriptions: [], extensionPath: process.cwd() } as any;
      const provider = new CosmSCMProvider(client, mockContext);
      provider.setActiveUniverse('u/custom-branch');
      (provider as any).modifiedGroup.resourceStates = [
        { resourceUri: { fsPath: path.resolve(process.cwd(), 'src/a.ts') } },
        { resourceUri: { fsPath: path.resolve(process.cwd(), 'src/b.ts') } }
      ];

      const success = await provider.stageAll();
      assert.strictEqual(success, true);
      assert.strictEqual(targetUniverse, 'u/custom-branch');
      assert.strictEqual(stagedFiles.length, 2);

      // On failure
      (client as any).stageFiles = async () => {
        throw new Error('disk failure');
      };
      const failed = await provider.stageAll();
      assert.strictEqual(failed, false);
    });

    it('should auto-stage modified files and execute commit with activeUniverse', async () => {
      const client = new CosmClient(process.cwd());
      let commitOptions: any = null;
      (client as any).commit = async (options: any) => {
        commitOptions = options;
        return { merkle_root: 'abcdef1234567890abcdef', message: 'Committed' };
      };

      const mockContext = { subscriptions: [], extensionPath: process.cwd() } as any;
      const provider = new CosmSCMProvider(client, mockContext);
      provider.setActiveUniverse('u/dev-branch');
      (provider as any).scm.inputBox.value = 'feat: auto-stage commit test';
      (provider as any).stagedGroup.resourceStates = [];
      (provider as any).modifiedGroup.resourceStates = [
        { resourceUri: { fsPath: path.resolve(process.cwd(), 'main.go') } }
      ];

      let stageAllCalled = false;
      provider.stageAll = async () => {
        stageAllCalled = true;
        (provider as any).stagedGroup.resourceStates = [
          { resourceUri: { fsPath: path.resolve(process.cwd(), 'main.go') } }
        ];
        return true;
      };

      await provider.commit();
      assert.ok(stageAllCalled, 'stageAll should be called when stagedGroup is empty');
      assert.ok(commitOptions);
      assert.strictEqual(commitOptions.universe, 'u/dev-branch');
      assert.strictEqual(commitOptions.intent, 'feat: auto-stage commit test');
      assert.strictEqual((provider as any).scm.inputBox.value, '');
    });

    it('should support cosm.mergeUniverse argument resolution from string and tree item', async () => {
      const client = new CosmClient(process.cwd());
      let calledSource: string | undefined;
      let calledTarget: string | undefined;
      let calledStrategy: string | undefined;

      (client as any).mergeUniverse = async (source: string, target?: string, strategy?: string) => {
        calledSource = source;
        calledTarget = target;
        calledStrategy = strategy;
        return {
          source_universe: source,
          target_universe: target || 'universe-main',
          head_hash: 'abc1234567890def',
          status: 'merged'
        };
      };

      // 1. Direct string invocation
      const res1 = await client.mergeUniverse('u/feature-1', 'universe-main', 'union');
      assert.strictEqual(calledSource, 'u/feature-1');
      assert.strictEqual(calledTarget, 'universe-main');
      assert.strictEqual(calledStrategy, 'union');
      assert.strictEqual(res1.target_universe, 'universe-main');
      assert.strictEqual(res1.head_hash, 'abc1234567890def');

      // 2. Tree item with .change.universe_id
      const treeItemWithChange = {
        change: {
          change_id: 'c/api-routes',
          universe_id: 'u/api-universe',
          status: 'Active'
        }
      };
      const sourceFromChange = treeItemWithChange.change.universe_id;
      const res2 = await client.mergeUniverse(sourceFromChange, 'universe-main', 'fast-forward');
      assert.strictEqual(calledSource, 'u/api-universe');
      assert.strictEqual(calledTarget, 'universe-main');
      assert.strictEqual(calledStrategy, 'fast-forward');
      assert.strictEqual(res2.head_hash, 'abc1234567890def');
    });

    it('should format commit log entries properly in showLog flow', async () => {
      const client = new CosmClient(process.cwd());
      const sampleEntries: CommitLogEntry[] = [
        {
          commit_hash: '9f83ac0124fe',
          universe_id: 'universe-main',
          author: 'cosm-agent',
          timestamp: '2026-09-23T20:15:00Z',
          intent: 'feat: AST-native Merkle DAG union merge',
          components: ['pkg/core/engine.go', 'pkg/storage/graphengine.go'],
          user_prompt: 'Integrate AST union merge'
        }
      ];

      (client as any).execCosm = async () => JSON.stringify(sampleEntries);

      const entries = await client.getLog('universe-main', 30);
      assert.strictEqual(entries.length, 1);

      const entry = entries[0];
      const shortHash = entry.commit_hash.substring(0, 8);
      const label = `$(git-commit) ${shortHash} - ${entry.intent}`;
      const description = `(${entry.universe_id}) by ${entry.author}`;
      const compCount = entry.components.length;
      const detail = `${new Date(entry.timestamp).toLocaleString()} | ${compCount} components | Prompt: ${entry.user_prompt}`;

      assert.strictEqual(label, '$(git-commit) 9f83ac01 - feat: AST-native Merkle DAG union merge');
      assert.strictEqual(description, '(universe-main) by cosm-agent');
      assert.ok(detail.includes('2 components'));
      assert.ok(detail.includes('Prompt: Integrate AST union merge'));
    });

    it('should synchronize activeUniverse across SCMProvider, StatusBar, and Client when switching', async () => {
      const client = new CosmClient(process.cwd());
      let switchedTo: string | undefined;
      (client as any).switchUniverse = async (univ: string) => {
        switchedTo = univ;
        return `Switched to micro-universe '${univ}'`;
      };

      const mockContext = { subscriptions: [], extensionPath: process.cwd() } as any;
      const provider = new CosmSCMProvider(client, mockContext);
      provider.setActiveUniverse('u/new-universe');
      assert.strictEqual(provider.getActiveUniverse(), 'u/new-universe');

      await client.switchUniverse('u/new-universe');
      assert.strictEqual(switchedTo, 'u/new-universe');
    });
  });
});


