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
The Perry Go orchestrator will route requests to different local IP addresses:
- `http://<linux-box-ip>:11434` -> Coder Tasks
- `http://<khadas-mind-ip>:11434` -> Auditor/Architect Tasks

## Tuning Observations & Performance Trade-offs

### 1. The VRAM vs. Context (KV Cache) Pressure
Increasing `num_ctx` to 32k or 64k significantly increases the **KV Cache** footprint in VRAM. 
- **Observation:** A 32b model (Q4_K_M) takes ~18-20GB of raw weights, but fits in 16GB VRAM via partial offloading. 
- **The "Context Wall":** At 32k context, the KV cache can add 2-4GB of additional VRAM pressure. If the combined weights + cache exceed 16GB, Ollama will offload more layers to System RAM (CPU), causing a "speed cliff" where performance drops from ~10 t/s to ~2 t/s.
- **Tuning Tip:** If performance tanks at 32k, try dropping to **24,576 (24k)** or switching to a lighter quantization (`Q4_K_S` or `Q3_K_M`) to keep the "Active Working Set" inside the GPU.

### 2. Quantization Strategy for the "Sovereign Cluster"
- **32b Models (Sweet Spot):** Use `Q4_K_M`. This is the best balance of logic retention and speed for the 4070 Ti / 4060 class cards.
- **70b Models (The Genius Tier):** Use `Q3_K_S`. Even on 64GB of system RAM, a 70b model is "heavy." Lower quantization allows more of the model to stay in RAM without hitting swap, which is fatal for performance. Use this strictly for "Architectural Deadlocks" where reasoning is more important than speed.

### 3. Offloading Background Tasks (Arc & NPU)
To keep the main NVIDIA GPUs 100% available for the Coder/Auditor, we use the Khadas Mind's secondary silicon:
- **Arc iGPU (OpenVINO):** Perfect for **Llama-Guard** or **ShieldGemma**. These models act as "Input/Output Firewalls" to catch prompt injections or malicious output before they hit the Discord UI.
- **NPU (34 TOPS):** Best used for **Local Embeddings** (e.g., `bge-m3` or `nomic-embed-text`). By running the vectorization on the NPU, the Perry "Sovereign Cache" can index your codebase in the background without stealing a single CUDA core from the Coder.

## Deployment Checklist
1. [ ] `ollama pull qwen2.5-coder:32b` (Node 1)
2. [ ] `ollama pull deepseek-r1:32b` (Node 2)
3. [ ] Create `perry-cluster` with 32k `num_ctx` on both.
4. [ ] Run `nvtop` on Linux and `Intel GPU Top` on Khadas to verify offloading.
5. [ ] **Verification:** Run a 200-line code file through the Coder and monitor VRAM "Memory" % to ensure no OOM (Out of Memory) crashes.
