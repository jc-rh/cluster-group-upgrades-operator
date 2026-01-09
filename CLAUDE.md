# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Key Commands

### Building and Development
```bash
# Build the operator binary
make build

# Run the operator locally (requires KUBECONFIG set to cluster with RHACM)
export KUBECONFIG=/path/to/kubeconfig
make install run

# Generate code and manifests
make generate
make manifests

# Format and vet code
make fmt vet

# Run static analysis
make golangci-lint
```

### Testing
```bash
# Run unit tests
make unittests

# Run all CI checks (generates, formats, lints, tests)
make ci-job

# Run comprehensive tests (requires cluster setup)
make test

# Run a single test
go test -v ./controllers/... -run TestSpecificFunction
```

### Deployment
```bash
# Build and deploy operator image
make docker-build docker-push deploy IMG=your-registry/image:tag

# Deploy with pre-caching enabled
PRECACHE_IMG=your-registry/precache:tag make docker-build-precache docker-push-precache
make deploy IMG=your-registry/image:tag PRECACHE_IMG=your-registry/precache:tag

# Deploy with recovery enabled
RECOVERY_IMG=your-registry/recovery:tag make docker-build-recovery docker-push-recovery
make deploy IMG=your-registry/image:tag RECOVERY_IMG=your-registry/recovery:tag
```

### Linting and Code Quality
```bash
# Run all linters
make lint

# Individual linter commands
make golangci-lint    # Go static analysis
make shellcheck       # Shell script linting
make yamllint         # YAML linting
make bashate          # Bash script style checking
```

## Architecture Overview

### Core Components

**ClusterGroupUpgrade Controller** (`controllers/clustergroupupgrade_controller.go`)
- Main reconciliation logic for managing cluster upgrade workflows
- Handles three rollout types: Policy, ManifestWork, and Mixed (newly added)
- Manages batched upgrades with canaries, timeouts, and progress tracking
- State machine: ClustersSelected → Validated → PrecacheSpecValid → PrecachingSucceeded → BackupSucceeded → Progressing → Succeeded

**ManagedClusterForCGU Controller** (`controllers/managedclusterForCGU_controller.go`) 
- Automatically creates ClusterGroupUpgrade CRs for Zero Touch Provisioning (ZTP) workflows
- Monitors ManagedCluster Ready state and applies "ztp-done" labels

### Key CRDs

**ClusterGroupUpgrade** (`pkg/api/clustergroupupgrades/v1alpha1/types.go`)
- Primary CR defining upgrade specifications for cluster groups
- Supports mixed mode: both ACM policies and ManifestWork templates in single CR
- Key fields: clusters, managedPolicies, manifestWorkTemplates, remediationOrder (custom sequencing)
- Three rollout types determine execution strategy

**PreCachingConfig** 
- Defines container image pre-caching configurations
- Referenced by ClusterGroupUpgrade for optimization

### Rollout Types & Mixed Mode

The operator supports three execution modes:

1. **Policy Mode**: Uses ACM policies with placement rules
2. **ManifestWork Mode**: Uses ManifestWork resources directly  
3. **Mixed Mode**: Combines both policies and ManifestWorks with custom ordering

**Mixed Mode Implementation** (recently added):
- `RemediationItem` struct unifies policy and ManifestWork tracking
- `RemediationOrder` field allows custom sequencing (backup → pause → upgrade → resume → validate)
- Unified progress tracking with `ItemIndex` (backward compatible with legacy `PolicyIndex`/`ManifestWorkIndex`)
- Controller logic in `getClusterProgressMixed()` handles unified compliance checking

### Key Controller Patterns

**Progress Tracking** (`controllers/clustergroupupgrade_controller.go`)
- `updateClusterProgress()`: Updates remediation progress per cluster
- `getClusterProgress()`: Checks compliance and determines next actions
- `buildRemediationPlan()`: Creates batched execution plan with canary support

**Validation** (`controllers/validation.go`)  
- `validateMixedMode()`: Validates mixed mode configurations
- `validateRemediationOrder()`: Ensures referenced policies/templates exist
- Policy dependency order validation

**Actions** (`controllers/actions.go`)
- `takeActionsBeforeEnable()`: Pre-upgrade cluster label/annotation management  
- `takeActionsAfterCompletion()`: Post-upgrade cleanup and labeling
- Cluster metadata management for tracking upgrade state

### Utility Modules

**Policy Management** (`controllers/policy.go`)
- ACM policy compliance checking and placement rule management
- Handles policy copying, enforcement, and cleanup
- Integrates with ACM governance framework

**ManifestWork Management** (`controllers/manifestwork.go`)
- Direct ManifestWork deployment and status tracking
- Alternative to policy-based approach for specific use cases

**Pre-caching** (`controllers/precache.go`)
- Container image pre-caching on target clusters before upgrades
- Reduces upgrade downtime by pre-positioning required images

**Backup & Recovery** (`controllers/backup.go`)
- Cluster backup capabilities before upgrades
- Integration points for recovery workflows

### Testing Structure

**Unit Tests**: Located alongside source files (`*_test.go`)
**Integration Tests**: `tests/kuttl/` directory with KUTTL test scenarios
**Test Utilities**: `tests/pkg/` contains shared test frameworks and client utilities

### Important Design Patterns

**Finite State Machine**: ClusterGroupUpgrade follows clear state transitions with condition-based status reporting

**Batched Execution**: Upgrades processed in configurable batches with canary cluster support for risk mitigation

**Backward Compatibility**: Legacy `PolicyIndex`/`ManifestWorkIndex` fields maintained with migration to unified `ItemIndex` approach

**Event-Driven**: Emits Kubernetes events for upgrade lifecycle tracking (CguCreated, CguStarted, CguSuccess, CguTimedout)

## Important Notes

- Advanced Cluster Management (ACM) operator v2.9+ required on hub cluster
- Operator designed for OpenShift CNF/5G edge cluster fleet management  
- Mixed mode allows sophisticated workflows combining governance (policies) with direct deployment (ManifestWorks)
- All deprecated field usage has golangci-lint exceptions with `//nolint:staticcheck` annotations for SA1019