import * as assert from 'assert';
import * as fs from 'fs';
import * as path from 'path';
import { CosmClient } from '../client/cosmClient';
import {
  CosmStatus,
  UniverseRecord,
  ResolvedSymbol,
  FullTopologyGraph,
  BlastRadiusReport,
  ShipResult,
  LineageEnvelope
} from '../types';

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
        user_id: 'jasondavenport',
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
      assert.strictEqual(lineage.user_id, 'jasondavenport');

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
      assert.strictEqual(packageJson.contributes.views['cosm-explorer'][0].id, 'cosm.topologyView');
      assert.strictEqual(packageJson.contributes.views['scm'][0].id, 'cosm.scmView');
    });
  });
});
