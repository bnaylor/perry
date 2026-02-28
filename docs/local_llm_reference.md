# Reference: Local LLM Cluster Configuration (Phase 2)

## Hardware Profile: The "Perry Cluster"
We are moving from a single-node setup to a **Dual-Node Local Cluster** to allow simultaneous execution of high-reasoning agents.

### Node 1: The "Heavy" ("diffuser", Linux box)
- **Specs:** Ryzen 9, 64GB RAM, RTX 4070 Ti Super (16GB VRAM)
- **Primary Role:** **The Coder / Primary Architect**
- **Model:** `qwen2.5-coder:32b` (32k-64k Context)
- **Rationale:** Highest CUDA throughput for rapid code generation.

### Node 2: The "Brain" (Khadas Mind 2, Linux box)
- **Specs:** Intel Core Ultra 7, 64GB RAM, RTX 4060 (16GB VRAM)
- **Primary Role:** **The Shadow Auditor / Secondary Architect**
- **Model:** `deepseek-r1:32b` (32k Context)
- **Special Ops:** 
    - **Arc iGPU:** Running OpenVINO-optimized `Llama-Guard` for real-time safety filtering.
    - **NPU (34 TOPS):** Offloading embedding generation for codebase indexing (Phase 3).

## Cluster Role Distribution (The Roundtable)
By using two nodes, we can run a "Parallel Roundtable" without VRAM contention.

| Task | Node 1 (Linux) | Node 2 (Khadas) |
| :--- | :--- | :--- |
| **Ideating** | Architect (Qwen 32b) | Architect (DeepSeek 32b) |
| **Auditing** | Coder (Qwen 32b) | Shadow Auditor (DeepSeek 32b) |
| **Execution** | Sandbox Runner | Notary / Result Scanner |

## Optimization: The 32k Context Fix
Ollama's default 4k context is insufficient. We are targeting **32,768 (32k)** context for all cluster nodes.

### Custom Modelfile (Permanent)
Create a `perry-cluster.Modelfile` on both nodes:
```dockerfile
FROM qwen2.5-coder:32b # or deepseek-r1:32b
PARAMETER num_ctx 32768
PARAMETER temperature 0.2
```
Then run: `ollama create perry-agent -f perry-cluster.Modelfile`

## Multi-Node Dispatching
The Perry Go orchestrator's `Dispatcher` must be updated to support named `provider_instances`. 

### `routing.yaml` Example (Proposed)
```yaml
providers:
  - name: local-cuda
    type: ollama
    base_url: "http://<linux-box-ip>:11434"
  - name: local-npu
    type: ollama
    base_url: "http://<khadas-mind-ip>:11434"

routing:
  roles:
    coder: local-cuda
    auditor: local-npu
    architect: [local-cuda, local-npu] # Round-robin or capability-based
```

## Tuning Observations & Performance Trade-offs

### 1. The VRAM vs. Context (KV Cache) Pressure
Increasing `num_ctx` to 32k or 64k significantly increases the **KV Cache** footprint in VRAM. 
- **The "Speed Cliff":** If combined weights + cache exceed 16GB, Ollama will offload more layers to System RAM, causing performance to drop from ~10 t/s to ~2 t/s.
- **Tuning Tip:** If performance tanks at 32k, try dropping to **24k** or using lighter quantization.

### 2. Quantization Strategy for the "Sovereign Cluster"
- **32b Models (Sweet Spot):** Use `Q4_K_M`. Best balance of logic retention and speed.
- **70b Models (The Genius Tier):** **Caveat:** Q3 quantization for 70b models (like Llama-3) often results in a significant loss of reasoning capability compared to higher-bit 32b models. Use these strictly for non-critical brainstorming. For high-stakes reasoning, prefer cloud escalation (Claude/Gemini).

### 3. Offloading Background Tasks (Arc & NPU)
To keep the main NVIDIA GPUs 100% available for the Coder/Auditor, we use the Khadas Mind's secondary silicon:
- **Arc iGPU (OpenVINO):** Optimized for **Llama-Guard** (Input/Output Safety Firewall).
- **NPU (34 TOPS):** Optimized for **Local Embeddings** (Codebase Indexing).

## Deployment Checklist
1. [ ] `ollama pull qwen2.5-coder:32b` (Node 1)
2. [ ] `ollama pull deepseek-r1:32b` (Node 2)
3. [ ] Create `perry-agent` with 32k `num_ctx` on both.
4. [ ] Run `scripts/setup-cluster.sh` (Auto-pulls models, creates Modelfiles, runs smoke tests).
5. [ ] **Verification:** Run a 200-line code file through the Coder and monitor VRAM % to ensure no OOM.
