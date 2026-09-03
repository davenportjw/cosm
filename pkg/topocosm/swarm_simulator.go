package topocosm

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

// SwarmSimulationConfig configures parameters for multi-agent concurrency testing.
type SwarmSimulationConfig struct {
	NumAgents   int           `json:"num_agents"`
	Concurrency int           `json:"concurrency"`
	Duration    time.Duration `json:"duration"`
	OrgSlug     string        `json:"org_slug"`
	CosmName    string        `json:"cosm_name"`
}

// SwarmSimulationResult collects metrics from the simulation run.
type SwarmSimulationResult struct {
	TotalOperations  int64         `json:"total_operations"`
	SparsePulls      int64         `json:"sparse_pulls"`
	ClaimsAcquired   int64         `json:"claims_acquired"`
	ClaimsContested  int64         `json:"claims_contested"`
	ProposalsCreated int64         `json:"proposals_created"`
	ProposalsMerged  int64         `json:"proposals_merged"`
	ErrorsCount      int64         `json:"errors_count"`
	Duration         time.Duration `json:"duration"`
	OpsPerSecond     float64       `json:"ops_per_second"`
}

// RunSwarmSimulation executes a multi-agent concurrent benchmark against a Topocosm Hub server.
func RunSwarmSimulation(ctx context.Context, server *HubServer, cfg SwarmSimulationConfig) (*SwarmSimulationResult, error) {
	if cfg.NumAgents <= 0 {
		cfg.NumAgents = 10
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 5
	}
	if cfg.Duration <= 0 {
		cfg.Duration = 5 * time.Second
	}
	if cfg.OrgSlug == "" {
		cfg.OrgSlug = "demo-org"
	}
	if cfg.CosmName == "" {
		cfg.CosmName = "cloud-platform"
	}

	result := &SwarmSimulationResult{}
	startTime := time.Now()

	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup

	ctxTimeout, cancel := context.WithTimeout(ctx, cfg.Duration)
	defer cancel()

	for i := 0; i < cfg.NumAgents; i++ {
		agentIndex := i + 1
		agentDID := fmt.Sprintf("did:key:z6MkuVirtualAgent%03d", agentIndex)

		wg.Add(1)
		go func(did string, id int) {
			defer wg.Done()

			client := NewInProcessHubClient(server.Handler(), did)
			defer client.Close()

			_ = client.Enroll(ctxTimeout, did, fmt.Sprintf("agent-%03d", id), fmt.Sprintf("agent%03d@topocosm.dev", id), fmt.Sprintf("Agent %03d", id), "mock-key", true)

			for {
				select {
				case <-ctxTimeout.Done():
					return
				default:
				}

				sem <- struct{}{}

				// Step 1: Sparse Subtree Pull
				_, err := client.SparsePullCosm(ctxTimeout, &SparsePullRequest{
					OrgSlug:        cfg.OrgSlug,
					CosmName:       cfg.CosmName,
					UniverseID:     "universe-main",
					ComponentNames: []string{"services/billing"},
				})
				if err == nil {
					atomic.AddInt64(&result.SparsePulls, 1)
					atomic.AddInt64(&result.TotalOperations, 1)
				} else {
					atomic.AddInt64(&result.ErrorsCount, 1)
				}

				// Step 2: Blackboard Claim
				domain := fmt.Sprintf("services/billing/shard-%d", id%3)
				acquired, err := client.ClaimDomain(ctxTimeout, cfg.OrgSlug, cfg.CosmName, domain, "Concurrency stress test mutation", 5)
				if err == nil && acquired {
					atomic.AddInt64(&result.ClaimsAcquired, 1)
					atomic.AddInt64(&result.TotalOperations, 1)

					// Step 3: Create Proposal
					lineage := core.LineageEnvelope{
						ExecutingAgentID: did,
						Intent:           fmt.Sprintf("Concurrent Agent %d change", id),
						Timestamp:        time.Now().UTC(),
					}
					propTitle := fmt.Sprintf("Auto-Proposal Agent %d-%d", id, time.Now().UnixNano()%1000)
					prop, err := client.CreateProposal(ctxTimeout, cfg.OrgSlug, cfg.CosmName, propTitle, "universe-main", "universe-main", lineage)
					if err == nil && prop != nil {
						atomic.AddInt64(&result.ProposalsCreated, 1)
						atomic.AddInt64(&result.TotalOperations, 1)

						// Step 4: Merge Proposal
						_, err = client.MergeProposal(ctxTimeout, cfg.OrgSlug, cfg.CosmName, prop.ID)
						if err == nil {
							atomic.AddInt64(&result.ProposalsMerged, 1)
							atomic.AddInt64(&result.TotalOperations, 1)
						}
					}

					// Release domain
					_ = client.ReleaseDomain(ctxTimeout, cfg.OrgSlug, cfg.CosmName, domain)
				} else {
					atomic.AddInt64(&result.ClaimsContested, 1)
				}

				<-sem
				time.Sleep(10 * time.Millisecond)
			}
		}(agentDID, agentIndex)
	}

	wg.Wait()
	result.Duration = time.Since(startTime)
	if result.Duration.Seconds() > 0 {
		result.OpsPerSecond = float64(result.TotalOperations) / result.Duration.Seconds()
	}

	return result, nil
}
