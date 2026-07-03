# Plan: Replace Deprecated `GetEventRecorderFor` API

## Context

The controller-runtime `Manager.GetEventRecorderFor(name)` method is deprecated in v0.23.3. It returns a `record.EventRecorder` (old `core/v1` events API). The replacement is `Manager.GetEventRecorder(name)`, which returns an `events.EventRecorder` (new `events/v1` API).

The project has two call sites with `//nolint: staticcheck` suppressing the deprecation warning. The new API has a different `Eventf` signature and **no `AnnotatedEventf` equivalent**.

### Deprecation Timeline

According to the [controller-runtime pkg/recorder documentation](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/recorder), `GetEventRecorderFor` is marked as "will be removed in a future release" but **no specific version or date** has been announced. The API remains functional in v0.23.3+, but migration is recommended to avoid future breakage.

### Annotation Support in New Events API

The `events.k8s.io/v1.Event` resource **does support annotations** through its embedded `metav1.ObjectMeta` field. However, the `EventRecorder` interface does not expose a method to set them. 

According to [KEP-383 (New Event API GA Graduation)](https://github.com/kubernetes/enhancements/blob/master/keps/sig-instrumentation/383-new-event-api-ga-graduation/README.md), the discussion about avoiding annotations was specifically about the **internal Event API migration strategy** (using annotations for backwards compatibility of deprecated fields), not about prohibiting operators from using annotations for business data.

This means we have **two valid migration approaches**:
1. **Use EventRecorder interface** (simple) — fold annotation data into structured fields (`action`, `note`)
2. **Use EventSink directly** (verbose) — preserve annotations for external tool compatibility

The choice depends on whether external tools currently rely on reading `cgu.openshift.io/*` annotations from events.

## Migration Approach Decision

**Choose ONE of the following approaches:**

### Approach A: EventRecorder Interface (Recommended for Simplicity)

Use the standard `EventRecorder` interface and fold annotation data into structured fields.

**Pros:**
- Uses the intended controller-runtime interface
- Simpler code, less boilerplate
- Aligns with Kubernetes Event API design (structured fields over unstructured annotations)

**Cons:**
- External tools reading `cgu.openshift.io/*` annotations from core/v1 Events will break
- Structured data becomes human-readable text in `note` field, not machine-parseable
- Must parse note strings to extract data programmatically

**Use this approach if:** No external tools currently rely on parsing event annotations.

### Approach B: EventSink Direct Access (Preserves Annotations)

Use `EventSink` directly to create events with annotations, bypassing the `EventRecorder` interface.

**Pros:**
- Preserves annotations for external tool compatibility
- No information loss during migration
- Structured data remains machine-readable

**Cons:**
- More verbose API (manual event construction)
- Must manage event naming, deduplication logic manually
- Bypasses controller-runtime abstraction layer
- Requires adding Kubernetes clientset to reconciler

**Use this approach if:** External tools currently read `cgu.openshift.io/*` annotations and must continue working.

### Approach C: Hybrid

Use `EventRecorder` for most events, but use `EventSink` for specific events where annotations are needed for external integrations.

---

**This plan documents Approach A (EventRecorder interface).** If Approach B or C is chosen after investigation, this plan will need revision.

### Investigation: Do External Tools Use Annotations?

Before committing to an approach, investigate:

1. **Search codebase for event annotation readers:**
   ```bash
   git grep -i "cgu.openshift.io/event" --all-match
   git grep -i "event.*annotation" 
   ```

2. **Check documentation** for references to event annotations being consumed by external systems

3. **Search related repositories** (monitoring, dashboards, automation tools) for:
   - Event filtering by `cgu.openshift.io/*` annotations
   - Metric extraction from event annotations
   - Alert rules based on event annotations

4. **Ask stakeholders:** Does any monitoring/alerting/dashboard tooling parse CGU event annotations?

If no external dependencies are found, **Approach A is recommended** for simplicity and alignment with Kubernetes API design principles.

## Changes (Approach A: EventRecorder Interface)

### 1. Remove dead code from `IBGUReconciler`

**File:** `controllers/imagebasedgroupupgrade.go`

The `Recorder` field is declared and initialized but never used anywhere in the IBGU controller.

- Remove `Recorder record.EventRecorder` from the struct (line 57)
- Remove `r.Recorder = mgr.GetEventRecorderFor("IBGU Reconciler")` and `//nolint: staticcheck` from `SetupWithManager` (line 480)
- Remove the `"k8s.io/client-go/tools/record"` import (line 34) — verify no other usage in the file first

### 2. Change CGU Recorder field type

**File:** `controllers/clustergroupupgrade_controller.go`

- Change field `Recorder record.EventRecorder` (line 61) to `Recorder events.EventRecorder`
- Add import `"k8s.io/client-go/tools/events"`; remove `"k8s.io/client-go/tools/record"` if no longer used
- Change `mgr.GetEventRecorderFor("ClusterGroupUpgrade")` (line 1421) to `mgr.GetEventRecorder("ClusterGroupUpgrade")`
- Remove the `//nolint: staticcheck` comment

### 3. Rewrite event-sending methods

**File:** `controllers/events.go`

Convert every `r.Recorder.AnnotatedEventf(cgu, evAnns, eventType, reason, message)` call to `r.Recorder.Eventf(cgu, related, eventType, reason, action, note)`.

**Parameter mapping:**
| Old param | New param | Source |
|-----------|-----------|--------|
| `object` (cgu) | `regarding` (cgu) | Same |
| — | `related` | `nil` for global/batch events; `ManagedCluster` object for cluster-level events |
| `eventtype` | `eventtype` | Same (`corev1.EventTypeNormal`/`Warning`) |
| `reason` | `reason` | Same (`CguCreated`, etc.) |
| — | `action` | From annotation `event-type` value, in UpperCamelCase: `"GlobalUpgrade"`, `"BatchUpgrade"`, `"ClusterUpgrade"`, `"Validate"` |
| `messageFmt` | `note` | Existing message, enriched with key annotation data (counts, cluster lists) |

**`related` for cluster-level events:** The two cluster-level methods (`sendEventCGUClusterUpgradeStarted`, `sendEventCGUClusterUpgradeSuccess`) already receive `clusterName string`. Construct a minimal `ManagedCluster` reference — the events API only reads the object reference (GVK + name), no API call needed:
```go
related := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: clusterName}}
r.Recorder.Eventf(cgu, related, ...)
```

**Per-method conversion:**

1. `sendEventCGUCreated` — related=`nil`, action=`"GlobalUpgrade"`, note=existing message
2. `sendEventCGUStarted` — related=`nil`, action=`"GlobalUpgrade"`, note=message + `(totalClusters=%d, totalBatches=%d)`
3. `sendEventCGUSuccess` — related=`nil`, action=`"GlobalUpgrade"`, note=message + `(totalClusters=%d)`
4. `sendEventCGUTimedout` — related=`nil`, action=`"GlobalUpgrade"`, note=message + `(totalClusters=%d, timedoutClusters=%d, timedoutClustersList=%s)`
5. `sendEventCGUBatchUpgradeStarted` — related=`nil`, action=`"BatchUpgrade"`, note=message + `(batchClusters=%d, totalClusters=%d, clustersList=%s)`
6. `sendEventCGUBatchUpgradeSuccess` — related=`nil`, action=`"BatchUpgrade"`, note=message + `(batchClusters=%d, totalClusters=%d, clustersList=%s)`
7. `sendEventCGUBatchUpgradeTimedout` — related=`nil`, action=`"BatchUpgrade"`, note=message + `(timedoutClusters=%d, batchClusters=%d, totalClusters=%d, timedoutClustersList=%s)`
8. `sendEventCGUClusterUpgradeStarted` — related=`ManagedCluster{Name: clusterName}`, action=`"ClusterUpgrade"`, note=existing message
9. `sendEventCGUClusterUpgradeSuccess` — related=`ManagedCluster{Name: clusterName}`, action=`"ClusterUpgrade"`, note=existing message
10. `sendEventCGUValidationFailureMissingClusters` — related=`nil`, action=`"Validate"`, note=existing message (cluster names already in message)
11. `sendEventCGUVPoliciesValidationFailure` — related=`nil`, action=`"Validate"`, note=existing message (policy names already in message)

**Truncation:** The `events/v1.Event.Note` field has a 1kB soft limit. Replace `truncateAnnotations` with a `truncateNote` function that caps the note string. `truncateListString` can be reused for truncating cluster/policy lists before they go into the note.

### 4. Clean up annotation constants and utilities

**File:** `controllers/events.go`

- Remove all `CGUEventAnnotationKey*` constants (lines 49-67)
- Remove `CGUAnnEvent*` constants (lines 70-74) — repurpose as action constants in UpperCamelCase
- Replace `truncateAnnotations` with `truncateNote` that caps the note string at `maxEventNoteSize` (1024 bytes)
- Keep `truncateListString` (still useful for truncating cluster lists in the note)
- Remove the commented-out `sendEventCGUClusterUpgradeTimedout` function (lines 276-290)
- Change `maxEventAnnsSize` to `maxEventNoteSize = 1024`

### 5. Update unit test files

**File:** `controllers/validation_test.go`
- Change all three `Recorder record.EventRecorder` field declarations (lines 203, 318, 848) to `Recorder events.EventRecorder`
- Change import from `"k8s.io/client-go/tools/record"` to `"k8s.io/client-go/tools/events"` (line 17)
- `nil` is still valid for the new interface type, so no logic changes needed

**File:** `controllers/events_test.go`
- Rewrite `Test_truncateAnnotations` to test the new `truncateNote` function
- Keep `Test_truncateListString` as-is

### 6. Update KUTTL integration test assertions

There are **29 Event assertions across 12 YAML files** in 3 KUTTL test suites that all use `core/v1` Event structure and must be rewritten for `events.k8s.io/v1`.

**Affected test suites (all under `tests/kuttl/cgu/`):**
- `upgrade-complete/` — `00-assert.yaml`, `01-assert.yaml`, `02-assert.yaml`, `03-assert.yaml`
- `upgrade-batch-timeout/` — `00-assert.yaml`, `01-assert.yaml`, `02-assert.yaml`, `03-assert.yaml`
- `upgrade-partial-batch-timeout/` — `00-assert.yaml`, `01-assert.yaml`, `03-assert.yaml`, `04-assert.yaml`

**Field mapping for each Event assertion:**

| core/v1 Event (old) | events.k8s.io/v1 Event (new) |
|---|---|
| `apiVersion: v1` | `apiVersion: events.k8s.io/v1` |
| `kind: Event` | `kind: Event` (same) |
| `involvedObject: {apiVersion, kind, name, namespace}` | `regarding: {apiVersion, kind, name, namespace}` |
| `message: "..."` | `note: "..."` (enriched with former annotation data) |
| `reason: CguStarted` | `reason: CguStarted` (same) |
| `type: Normal` | `type: Normal` (same) |
| `count: 1` | remove (replaced by `series` field, not asserted) |
| `reportingComponent: ClusterGroupUpgrade` | `reportingController: ClusterGroupUpgrade` |
| `reportingInstance: ""` | `reportingInstance: ""` (same) |
| `source: {component: ClusterGroupUpgrade}` | remove (not used in events/v1) |
| `metadata.annotations: {cgu.openshift.io/*}` | remove (data moved into `note` and `action` fields) |
| — | `action: GlobalUpgrade` / `BatchUpgrade` / `ClusterUpgrade` (new field) |

For cluster-level events, add:
| — | `related: {apiVersion: cluster.open-cluster-management.io/v1, kind: ManagedCluster, name: <clusterName>}` |

**Note field changes:** The `note` field must match the enriched message format from step 3. For example, the old assertion:
```yaml
message: ClusterGroupUpgrade cgu started remediating policies
metadata:
  annotations:
    cgu.openshift.io/event-type: global
    cgu.openshift.io/total-clusters-count: "2"
    cgu.openshift.io/total-batches-count: "2"
```
becomes:
```yaml
note: "ClusterGroupUpgrade cgu started remediating policies (totalClusters=2, totalBatches=2)"
action: GlobalUpgrade
```

### 7. Verify imports are clean

After all changes, confirm:
- `controllers/clustergroupupgrade_controller.go` — imports `events`, not `record`
- `controllers/imagebasedgroupupgrade.go` — no longer imports `record`
- `controllers/events.go` — no `record` import needed (was not imported here)
- `controllers/validation_test.go` — imports `events`, not `record`

## Verification

1. `make fmt` — ensure formatting is correct
2. `make golangci-lint` — no linting errors, no more `//nolint: staticcheck` needed
3. `make unittests` — all unit tests pass including updated `events_test.go` and `validation_test.go`
4. `make build` — project compiles cleanly
5. KUTTL tests — the 12 updated assertion files should match the new `events.k8s.io/v1` Event structure (requires RHACM cluster to run: `make complete-deployment && make kuttl-test`)

## Notes

- The migration switches from `core/v1.Event` to `events/v1.Event`. These are different API resources.
- With **Approach A** (EventRecorder): External tooling that reads `cgu.openshift.io/*` annotations from core/v1 Events will no longer find them — the structured data is now in the event's `note` and `action` fields instead.
- With **Approach B** (EventSink): Annotations are preserved, but custom event creation logic must be implemented.
- `kubectl get events` shows both event types by default in modern Kubernetes.
- The KUTTL assertion changes are the largest part of this migration by file count (12 files, 29 Event objects), but each follows the same mechanical field mapping described above.

## Appendix: Approach B Implementation (EventSink)

If Approach B is chosen, the following changes would replace steps 2-3:

### Add Kubernetes Clientset to Reconciler

**File:** `controllers/clustergroupupgrade_controller.go`

```go
type ClusterGroupUpgradeReconciler struct {
    client.Client
    Log       logr.Logger
    Scheme    *runtime.Scheme
    Recorder  events.EventRecorder  // Use new API
    Clientset kubernetes.Interface   // Add for EventSink access
}
```

In `SetupWithManager` (line ~1421):

```go
func (r *ClusterGroupUpgradeReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Create Kubernetes clientset
    clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
    if err != nil {
        return err
    }
    r.Clientset = clientset
    r.Recorder = mgr.GetEventRecorder("ClusterGroupUpgrade")
    
    // ... rest of setup
}
```

### Create EventSink Helper

**File:** `controllers/events.go`

```go
import (
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/events"
    eventsv1 "k8s.io/api/events/v1"
)

func (r *ClusterGroupUpgradeReconciler) createEventWithAnnotations(
    ctx context.Context,
    cgu *ranv1alpha1.ClusterGroupUpgrade,
    related *corev1.ObjectReference,
    eventType, reason, action, note string,
    annotations map[string]string,
) error {
    eventSink := &events.EventSinkImpl{
        Interface: r.Clientset.EventsV1(),
    }
    
    event := &eventsv1.Event{
        ObjectMeta: metav1.ObjectMeta{
            Name:        fmt.Sprintf("%s.%x", cgu.Name, time.Now().UnixNano()),
            Namespace:   cgu.Namespace,
            Annotations: annotations,
        },
        EventTime:           metav1.NewMicroTime(time.Now()),
        ReportingController: "ClusterGroupUpgrade",
        ReportingInstance:   os.Getenv("POD_NAME"), // or instance identifier
        Action:              action,
        Reason:              reason,
        Type:                eventType,
        Regarding: corev1.ObjectReference{
            APIVersion: cgu.APIVersion,
            Kind:       cgu.Kind,
            Name:       cgu.Name,
            Namespace:  cgu.Namespace,
            UID:        cgu.UID,
        },
    }
    
    if related != nil {
        event.Related = related
    }
    
    if note != "" {
        event.Note = note
    }
    
    _, err := eventSink.Create(ctx, event)
    return err
}
```

### Rewrite Event Methods

Each `sendEvent*` method would call `createEventWithAnnotations` instead of `r.Recorder.AnnotatedEventf`, preserving the original annotation structure.

### Additional Imports

Add to imports:
```go
"k8s.io/client-go/kubernetes"
"k8s.io/client-go/tools/events"
eventsv1 "k8s.io/api/events/v1"
```

## References

- [KEP-383: New Event API GA Graduation](https://github.com/kubernetes/enhancements/blob/master/keps/sig-instrumentation/383-new-event-api-ga-graduation/README.md) - Design rationale for the new events.k8s.io/v1 API
- [controller-runtime pkg/recorder documentation](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/recorder) - GetEventRecorderFor deprecation notice
- [k8s.io/client-go/tools/events package](https://pkg.go.dev/k8s.io/client-go/tools/events) - New EventRecorder interface and EventSink
- [k8s.io/client-go/tools/record package](https://pkg.go.dev/k8s.io/client-go/tools/record) - Old (deprecated) EventRecorder with AnnotatedEventf
- [k8s.io/api/events/v1 Event type](https://pkg.go.dev/k8s.io/api/events/v1) - events.k8s.io/v1 Event resource definition
