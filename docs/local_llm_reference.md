# Reference: Local LLM Cluster Configuration (Phase 2)

**Last Updated:** March 2, 2026

## Hardware Profile: The "Perry Cluster"
We use a **Dual-Node Local Cluster** to distribute the workload between code generation and adversarial auditing.

### Node 1: The "Heavy" ("diffuser", Linux box)
- **IP:** `10.3.2.8`
- **Specs:** Ryzen 9, 64GB RAM, RTX 4070 Ti Super (16GB VRAM)
- **Primary Role:** **The Coder / Strategist**
- **Models:** 
  - `qwen3:14b` (Primary Coder)
  - `qwen2.5-coder:14b` (Semantic Auditor)
- **Rationale:** High CUDA throughput for rapid code generation and initial semantic analysis.

### Node 2: The "Brain" ("mink", Linux box)
- **IP:** `10.3.2.48`
- **Specs:** Ryzen 7, 64GB RAM, RTX 4060 Ti (16GB VRAM)
- **Primary Role:** **The Shadow Auditor (Adversarial)**
- **Model:** `deepseek-r1:14b`
- **Special Ops:** 
    - **Service Config:** Ollama is configured to listen on `0.0.0.0` (via `/etc/systemd/system/ollama.service.d/override.conf`) to allow cluster-wide access.

## Cluster Role Distribution (The Roundtable)
By using two nodes, we run the "Parallel Roundtable" without VRAM contention on a single card.

| Role | Provider | Node | Model |
| :--- | :--- | :--- | :--- |
| **Strategist** | cloud-gemini | Cloud | `gemini-3-flash-preview` |
| **Researcher** | cloud-gemini | Cloud | `gemini-3-flash-preview` |
| **Coder** | local-cuda | Node 1 (diffuser) | `qwen3:14b` |
| **Auditor (Semantic)** | local-cuda | Node 1 (diffuser) | `qwen2.5-coder:14b` |
| **Auditor (Shadow)** | local-npu | Node 2 (mink) | `deepseek-r1:14b` |

## Verification & Discovery
The `perry` binary now includes built-in tools for verifying cluster readiness.

### 1. Model Listing
Use the `--models` flag to see all available models across the cluster:
```bash
./bin/perry --models
```
This confirms connectivity to both `local-cuda` (Node 1) and `local-npu` (Node 2) and lists their active models and capabilities.

### 2. Connectivity Troubleshooting
If a node is unreachable:
1. **Ping:** Check network connectivity (`ping 10.3.2.48`).
2. **Service Port:** Verify Ollama is running and listening (`curl http://10.3.2.48:11434/api/tags`).
3. **Listen Address:** Ensure `OLLAMA_HOST=0.0.0.0` is set in the systemd environment on the target node.

## Optimization: The Context Window
We target a **32k** context window for local models to handle large codebase snapshots (Mirror Cage).

### Custom Modelfile (Permanent)
To ensure consistent context across restarts, create a custom model:
```dockerfile
FROM qwen3:14b
PARAMETER num_ctx 32768
PARAMETER temperature 0.2
```
Then run: `ollama create perry-coder -f Modelfile`

## Deployment Checklist
1. [x] Node 1: `ollama pull qwen3:14b`
2. [x] Node 1: `ollama pull qwen2.5-coder:14b`
3. [x] Node 2: `ollama pull deepseek-r1:14b`
4. [x] Node 2: Configure `OLLAMA_HOST=0.0.0.0`.
5. [x] **Verification:** Run `./bin/perry --models` and ensure all three providers respond.
