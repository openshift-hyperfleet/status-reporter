# Status Reporter

A **cloud-agnostic**, **reusable** Kubernetes sidecar container that monitors adapter operation results and updates Kubernetes Job status.

## Overview

The status reporter is a production-ready Kubernetes sidecar container that works with any adapter container (validation, DNS, pull secret, etc.) that follows the defined result contract. It provides robust monitoring and status reporting capabilities for Kubernetes Jobs.

**Key Features:**
- Monitors adapter container execution via file polling and container state watching
- Handles various failure scenarios (OOMKilled, crashes, timeouts, invalid results)
- Updates Kubernetes Job status with detailed condition information
- Zero-dependency on adapter implementation - uses simple JSON contract
- Cloud-agnostic design - works with any Kubernetes environment

## Adapter Contract

The status reporter works with any adapter container that follows this simple JSON contract:

1. **Result File Requirements:**
    - **Location:** Write results to the result file (configurable via `RESULTS_PATH` env var)
    - **Format:** Valid JSON file (max size: 1MB)
    - **Timing:** Must be written before the adapter container exits or within the configured timeout

2. **JSON Schema:**
   ```json
   {
     "status": "success",           // Required: "success" or "failure"
     "reason": "AllChecksPassed",   // Required: Machine-readable identifier (max 128 chars)
     "message": "All validation checks passed successfully",  // Required: Human-readable description (max 1024 chars)
     "details": {                   // Optional: Adapter-specific data (any valid JSON), this information will not be reflected in k8s Job Status
       "checks_run": 5,
       "duration_ms": 1234
     }
   }
   ```

3. **Field Validation:**
    - `status`: Must be exactly `"success"` or `"failure"` (case-sensitive)
    - `reason`: Trimmed and truncated to 128 characters. Defaults to `"NoReasonProvided"` if empty/missing
    - `message`: Trimmed and truncated to 1024 characters. Defaults to `"No message provided"` if empty/missing
    - `details`: Optional JSON object containing any adapter-specific information

4. **Examples:**

   **Success result:**

   Adapter writes to the result file:
   ```json
   {
     "status": "success",
     "reason": "ValidationPassed",
     "message": "GCP environment validated successfully"
   }
   ```

   Resulting Kubernetes Job status:
   ```yaml
   status:
     conditions:
     - type: Available
       status: "True"
       reason: ValidationPassed
       message: GCP environment validated successfully
       lastTransitionTime: "2024-01-15T10:30:00Z"
   ```

   **Failure result with details:**

   Adapter writes to the result file:
   ```json
   {
     "status": "failure",
     "reason": "MissingPermissions",
     "message": "Service account lacks required IAM permissions",
     "details": {
       "missing_permissions": ["compute.instances.list", "iam.serviceAccounts.get"],
       "service_account": "my-sa@project.iam.gserviceaccount.com"
     }
   }
   ```

   Resulting Kubernetes Job status:
   ```yaml
   status:
     conditions:
     - type: Available
       status: "False"
       reason: MissingPermissions
       message: Service account lacks required IAM permissions
       lastTransitionTime: "2024-01-15T10:30:00Z"
   ```

   **Timeout scenario:**

   If adapter doesn't write result file within timeout, Job status will be:
   ```yaml
   status:
     conditions:
     - type: Available
       status: "False"
       reason: AdapterTimeout
       message: "Adapter did not produce results within 5m0s"
       lastTransitionTime: "2024-01-15T10:30:00Z"
   ```

   **Container crash scenario:**

   If adapter container exits with non-zero code, Job status will be:
   ```yaml
   status:
     conditions:
     - type: Available
       status: "False"
       reason: AdapterExitedWithError
       message: "Adapter container exited with code 1: Error"
       lastTransitionTime: "2024-01-15T10:30:00Z"
   ```

   **OOMKilled scenario:**

   If adapter container is killed due to memory limits:
   ```yaml
   status:
     conditions:
     - type: Available
       status: "False"
       reason: AdapterOOMKilled
       message: "Adapter container was killed due to out of memory (OOMKilled)"
       lastTransitionTime: "2024-01-15T10:30:00Z"
   ```

   **Invalid result format:**

   If adapter writes invalid JSON or schema:
   ```yaml
   status:
     conditions:
     - type: Available
       status: "False"
       reason: InvalidResultFormat
       message: "Failed to parse adapter result: status: must be either 'success' or 'failure'"
       lastTransitionTime: "2024-01-15T10:30:00Z"
   ```

5. **Shared Volume Configuration:**

   Both adapter and status reporter containers must share a volume mounted at `/results`:

   ```yaml
   volumes:
   - name: results
     emptyDir: {}

   containers:
   - name: adapter
     volumeMounts:
     - name: results
       mountPath: /results

   - name: status-reporter
     volumeMounts:
     - name: results
       mountPath: /results
   ```

## Configuration

The status reporter is configured exclusively through environment variables. Below is a complete reference of all supported configuration options.

### Environment Variables

| Environment Variable | Type | Required | Default | Description |
|---------------------|------|----------|---------|-------------|
| `JOB_NAME` | string | **Yes** | - | Name of the Kubernetes Job to update |
| `JOB_NAMESPACE` | string | **Yes** | - | Namespace of the Kubernetes Job |
| `POD_NAME` | string | **Yes** | - | Name of the current Pod (typically injected via downward API) |
| `RESULTS_PATH` | string | No | `/results/adapter-result.json` | Absolute path to the adapter result file (must be a file, not a directory) |
| `POLL_INTERVAL_SECONDS` | integer | No | `2` | Interval in seconds between result file checks (must be positive and less than MAX_WAIT_TIME_SECONDS) |
| `MAX_WAIT_TIME_SECONDS` | integer | No | `300` | Maximum time in seconds to wait for adapter results before timing out (must be positive) |
| `CONDITION_TYPE` | string | No | `Available` | Kubernetes condition type to set on the Job status |
| `LOG_LEVEL` | string | No | `info` | Logging verbosity level |
| `ADAPTER_CONTAINER_NAME` | string | No | `""` (auto-detect) | Name of the adapter container to monitor; if empty, automatically detects the first non-reporter container in the Pod |

### Configuration Example

Here's a complete example showing how to configure the status reporter. 
### RBAC configuration for Job Status Reporter
```yaml
---
# ServiceAccount for the validator job
apiVersion: v1
kind: ServiceAccount
metadata:
  name: status-reporter-sa
  namespace: <namespace>

---
# Role with necessary permissions for status reporter
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: status-reporter
  namespace: <namespace>
rules:
# Permission to get and update job status
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["get"]
- apiGroups: ["batch"]
  resources: ["jobs/status"]
  verbs: ["get", "update", "patch"]
# Permission to get pod status
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]

---
# RoleBinding to grant permissions to the service account
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: status-reporter
  namespace: <namespace>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: status-reporter
subjects:
- kind: ServiceAccount
  name: status-reporter-sa
  namespace: <namespace>
```
```bash
sed 's/<namespace>/your-namespace/g' rbac.yaml | kubectl apply -f -
```

### K8s Job configuration for Status Reporter
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: my-adapter-job
  namespace: <namespace>
spec:
  backoffLimit: 0
  activeDeadlineSeconds: 310  # 300 adapter timeout + 10 second buffer
  template:
    spec:
      serviceAccountName: status-reporter-sa   
      volumes:
      - name: results
        emptyDir: {}

      containers:
      # Replace with real adapter information
      - name: my-adapter
        image: busybox:1.36
        imagePullPolicy: IfNotPresent

        # Adapter writes results to shared volume
        volumeMounts:
        - name: results
          mountPath: /results

        # Simulate adapter work and write result file
        command:
        - /bin/sh
        - -c
        - |
          echo "Simulating adapter validation work..."
          sleep 3
          # Write adapter result in the expected JSON format
          # Success example:
          cat > $RESULTS_PATH <<'EOF'
          {
            "status": "success",
            "reason": "GCPValidationPassed",
            "message": "All GCP infrastructure validations completed successfully",
            "details": {
              "validations_run": 5,
              "validations_passed": 5,
              "checks": [
                "VPC configuration validated",
                "IAM permissions verified",
                "DNS settings confirmed",
                "Network policies applied",
                "Resource quotas checked"
              ],
              "duration_seconds": 3,
              "gcp_project": "example-project-123"
            }
          }
          EOF

          echo "Adapter result written to $RESULTS_PATH"
          cat $RESULTS_PATH
          sleep 5
        env:
        - name: RESULTS_PATH
          value: "/results/adapter-result.json"

      - name: status-reporter
        image: <status-reporter-image>
        env:
        # Required configuration
        - name: JOB_NAME
          value: my-adapter-job
        - name: JOB_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name

        # Optional configuration (shown with non-default values)
        - name: RESULTS_PATH
          value: "/results/adapter-result.json"
        - name: POLL_INTERVAL_SECONDS
          value: "5"
        - name: MAX_WAIT_TIME_SECONDS
          value: "300"  # 5 minutes
        - name: CONDITION_TYPE
          value: "Available"
        - name: LOG_LEVEL
          value: "info"
        - name: ADAPTER_CONTAINER_NAME
          value: "my-adapter"

        volumeMounts:
        - name: results
          mountPath: /results

      restartPolicy: Never
```

```bash
sed -e 's|<namespace>|your-namespace|g' \
    -e 's|<status-reporter-image>|quay.io/rh-ee-dawang/status-reporter:dev-04e8d0a|g' \
    job.yaml | kubectl apply -f -
```

## Repository Structure

```text
status-reporter/
├── cmd/reporter/         # Main entry point
├── pkg/                  # Core packages (reporter, k8s, result parser)
├── Dockerfile            # Container image definition
├── Makefile              # Build, test, and image targets
└── README.md             # This file
```

## Quick Start

The status reporter is production-ready and can be used with any adapter container.

### Makefile Usage

```bash
$ make
Available targets:
binary               Build binary
clean                Clean build artifacts and test coverage files
fmt                  Format code with gofmt and goimports
help                 Display this help message
image-dev            Build and push to personal Quay registry (requires QUAY_USER)
image-push           Build and push container image to registry
image                Build container image with Docker or Podman
lint                 Run golangci-lint
mod-tidy             Tidy Go module dependencies
test-coverage-html   Generate HTML coverage report
test-coverage        Run unit tests with coverage report
test                 Run unit tests with race detection
verify               Run all verification checks (lint + test)
```

## License

See LICENSE file for details.

## Contact

For questions or issues, please open a GitHub issue in this repository.
