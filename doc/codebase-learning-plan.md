# FlyBrain 源码精读学习计划 (Codebase Deep-Dive Plan)

本学习计划是继《感觉神经回路与行为映射学习手册》之后的**第二阶段（工程源码级精读）**。
核心目标是以**“职责边界划分（Responsibility Boundary）”**与**“像读故事一样理解任务编排（Story-driven Orchestration）”**的视角，自底向上、从后端到前端逐个文件吃透 FlyBrain 的工程实现。

---

## 整体精读路线图（共 6 站）

```mermaid
flowchart LR
    S1["✅ 第 1 站: server/caretaker.js<br>网关、事故检测与通信中枢"]:::done --> S2["✅ 第 2 站: server/db.js & ai.js<br>SQLite 存储与大模型适配"]:::done
    S2 --> S3["✅ 第 3 站: agent/run.sh & decide.js<br>自动化决策循环与策略"]:::done
    S3 --> S4["✅ 第 4 站: js/sim-worker.js & bridge<br>Worker 通信与脉冲聚合"]:::done
    S4 --> S5["✅ 第 5 站: js/connectome.js & constants<br>概念脑回路与驱动力计算"]:::done
    S5 --> S6["✅ 第 6 站: js/main.js & 渲染器<br>状态机、运动学与 UI 交互"]:::done
    classDef done fill:#2d6a4f,color:#fff
```

| 站次 | 目标文件 | 所属架构层 | 核心职责边界 | 当前进度 |
| :---: | :--- | :--- | :--- | :---: |
| **第 1 站** | [`server/caretaker.js`](../server/caretaker.js) | 应用网关层 (Application / Gateway) | 串联浏览器 WebSocket、`stdin` 控制流、`db.js` 存储与 `ai.js` 问答的总调度中枢 | ✅ 已完成 |
| **第 2 站** | [`server/db.js`](../server/db.js) & [`server/ai.js`](../server/ai.js) | 基础设施层 (Infrastructure) | SQLite 表结构设计、每日照料评分算法、多模型端点归一化与响应解析 | ✅ 已完成 |
| **第 3 站** | [`agent/run.sh`](../agent/run.sh), [`agent/decide.js`](../agent/decide.js), `*-policy.md` | 自动化代理层 (Agent / Policy) | 命名管道 (FIFO) 守护循环、恐惧退避 (Fear Backoff) 机制、LLM 决策与输出清洗 | ✅ 已完成 |
| **第 4 站** | [`js/sim-worker.js`](../js/sim-worker.js) & [`js/brain-worker-bridge.js`](../js/brain-worker-bridge.js) | 仿真引擎与桥接层 (Simulation & Bridge) | 13.9 万神经元 CSR 矩阵计算、Neuropil 门控休眠、真实脉冲向概念脑区的聚合映射 | ✅ 已完成 |
| **第 5 站** | [`js/connectome.js`](../js/connectome.js) & [`js/constants.js`](../js/constants.js) | 领域模型层 (Domain Model) | ~60 个概念神经群的突触传播、双缓冲状态切换、5 大内驱力更新与电机累加器 | ✅ 已完成 |
| **第 6 站** | [`js/main.js`](../js/main.js), [`js/fly-logic.js`](../js/fly-logic.js), `caretaker-*.js` | 表现与物理交互层 (Presentation & Physics) | 行为状态机仲裁、双时钟循环、标签页休眠保护、六足步态绘制与前后端 WebSocket 桥接 | ✅ 已完成 |

---

## 第 1 站笔记：[`server/caretaker.js`](../server/caretaker.js)（全长 415 行）

### 1. 角色定位与职责边界
`caretaker.js` 是后端的**总调度室（Hub / Gateway）**。它不直接编写复杂的 SQL 语句，也不直接处理底层环境变量解析，而是负责协调四个外部对象：
1. **浏览器前端**：通过 WebSocket 接收高频状态上报、下发控制指令；通过 HTTP 提供静态资源与报表 API。
2. **自动化 Agent (`run.sh`)**：通过 `stdin` 接收 JSON 控制命令，通过 `stdout` 输出执行回执与事件流。
3. **数据库模块 (`db.js`)**：委托其存储观测快照、动作记录、事故告警与聊天历史。
4. **AI 客户端模块 (`ai.js`)**：委托其调用大模型接口回答用户在侧边栏的提问。

### 2. 代码章节拆解（按故事线演进）

* **章节 1：初始化与核心状态变量（[第 1–22 行](../server/caretaker.js#L1-L22)）**
  * `lastState`：缓存最新一帧果蝇状态，供 `GET /state` 零延迟返回。
  * `lastObservationTime` & `OBSERVATION_INTERVAL_MS = 10000`：节流阀，限制每 10 秒向 SQLite 写入一次观测记录。
  * `lastActionTime`、`lastActionType`、`preFearLevel`：事故追责快照三元组，记录每次动作执行瞬间的恐惧基线。
  * `VALID_ACTIONS`：指令白名单 `['place_food', 'set_light', 'set_temp', 'touch', 'blow_wind', 'clear_food']`。

* **章节 2：感知 $\rightarrow$ 督察 $\rightarrow$ 控制的三大核心函数（[第 38–115 行](../server/caretaker.js#L38-L115)）**
  1. `handleStateMessage(data)`（[第 69–85 行](../server/caretaker.js#L69-L85)）：解析浏览器发来的状态 $\rightarrow$ 更新 `lastState` $\rightarrow$ 满 10 秒则调 `insertObservation` 落库 $\rightarrow$ 打印 `stdout` $\rightarrow$ 触发 `detectIncidents()`。
  2. `detectIncidents(state)`（[第 38–67 行](../server/caretaker.js#L38-L67)）：
     * **`scared_the_fly`（高危 `high`）**：动作发生后 5 秒内，`fear - preFearLevel > 0.2`，立刻落库并通过 WebSocket 广播告警，随后将 `lastActionTime` 置 `0` 避免重复触发。
     * **`forgot_to_feed`（中危 `medium`）**：当 `hunger > 0.9 && foodCount === 0` 时触发，带有 60 秒冷却期（查询上次同类事故时间戳）。
  3. `handleStdinCommand(line)`（[第 87–115 行](../server/caretaker.js#L87-L115)）：校验 JSON 与动作白名单 $\rightarrow$ 拍下 `preFearLevel` 快照 $\rightarrow$ 调 `insertAction` 落库并广播给前端活动流 $\rightarrow$ 通过 WebSocket 发送 `{ type: 'command', ... }` 驱动浏览器执行动作。

* **章节 3：AI 聊天上下文构建与请求处理（[第 117–184 行](../server/caretaker.js#L117-L184)）**
  * `buildChatContext()`：提取最近 20 条观测、20 条动作、20 条事故，拼接为带时间戳与精确数值的 Markdown 文本。
  * `handleChatRequest(userMessage, viewContext)`：将 `agent/chat-policy.md` + 遥测上下文 + 用户当前视图 `viewContext` 组装为 `systemPrompt`，附带最近 20 条对话历史调用 `anthropic.messages.create()`，并将问答双双落库。

* **章节 4：静态文件服务与安全防护（[第 186–239 行](../server/caretaker.js#L186-L239)）**
  * `serveStatic(req, res)`：托管项目根目录静态资源，通过剥离 `..` 并校验 `filePath.startsWith(STATIC_ROOT)` 防止目录穿越攻击（Path Traversal）。

* **章节 5 & 6：HTTP 路由表、WebSocket 生命周期与定时任务（[第 241–415 行](../server/caretaker.js#L241-L415)）**
  * **HTTP 路由**：`GET /state`、`POST /chat`、`GET /chat/history`、`GET /analytics/summary`、`GET /analytics/hunger-timeline`、`GET /calendar/scores`、`GET /calendar/day-activity`、`GET /activity/recent`。
  * **定时评分**：每 5 分钟调用 `caretakerDb.computeDailyScore(today)` 更新当日照料评分。
  * **优雅退出**：监听 `SIGINT` / `SIGTERM`，关闭数据库连接与 WebSocket Server。

### 3. 架构审视与潜在改进点
* **单 WebSocket 连接覆盖问题**：`var browserSocket = null;`（第 21、376 行）假设同一时刻只有一个浏览器标签页连接。若打开多个标签页，后连接的标签页会覆盖 `browserSocket`，且任一标签页关闭都会导致 `browserSocket = null`，使其他存留标签页失去指令接收能力（若需支持多客户端，可改为遍历 `wss.clients` 集合）。


---

## 第 2 站笔记：[`server/db.js`](../server/db.js) & [`server/ai.js`](../server/ai.js)

### 一、 [`server/db.js`](../server/db.js)（全长 381 行）—— 数据持久化仓储层

#### 1. 角色定位与职责边界
属于典型的 **Repository / Infrastructure 层**。对外仅暴露工厂函数 `openDb(dbPath)`，将所有 SQLite 表结构创建、WAL 并发控制、预编译语句与统计聚合完全封装在内部，使上层 `caretaker.js` 不含任何 SQL 逻辑。

#### 2. 核心设计拆解
* **WAL 并发模式（[第 72 行](../server/db.js#L72)）**：开启 `journal_mode = WAL`，确保高频的每 10 秒 `observations` 写入与前端并发的报表读取互不阻塞。
* **5 张核心表结构（[第 6–65 行](../server/db.js#L6-L65)）**：
  * `observations`：采用**“常用指标独立列 + `raw_data` 完整 JSON”**混合设计，兼顾 SQL 聚合性能与向后扩展性。
  * `actions` & `incidents`：在 [`getRecentActivity()`](../server/db.js#L165-L174) 中通过 `UNION ALL` 统一对齐为 7 列，单次查询即可返回混排的动态时间线。
  * `daily_scores`：在 `date` 列建立 `UNIQUE` 唯一索引，配合 `ON CONFLICT(date) DO UPDATE` 实现每日照料得分的原地更新（Upsert）。
  * `chat_messages`：存储侧边栏对话历史。
* **每日照料评分算法（[`computeDailyScore`](../server/db.js#L212-L249)）**：
  * 满分 100 分，扣除三项惩罚：
    1. 饥饿扣分：$\max(0, \overline{\text{hunger}} - 0.3) \times 40$（最高扣 28 分）；
    2. 惊吓扣分：$\min(\text{fearIncidents} \times 5, 30)$（每次扣 5 分，封顶 30 分）；
    3. 漏喂扣分：$\min(\text{forgotIncidents} \times 0.5, 20)$（每次扣 0.5 分，封顶 20 分）。
* **动态响应延迟与在线时长计算（[`getAnalyticsSummary`](../server/db.js#L251-L329)）**：
  * 响应延迟：匹配每次 `hunger > 0.7` 时刻与随后首次 `place_food` 时刻的时间差均值。
  * 在线时长：累加相邻两条 `observations` 间隔 $\le 60\text{s}$ 的时间片段，过滤离线空白期。

#### 3. 架构审视与潜在瑕疵
1. **`avg_response_time` 语义冲突**：`computeDailyScore`（[第 243 行](../server/db.js#L243)）将 `forgotIncidents`（漏喂次数整数）直接存入了 `daily_scores.avg_response_time` 列；而 `getAnalyticsSummary`（[第 280 行](../server/db.js#L280)）计算的是真实的喂食响应秒数。
2. **查询语句未缓存预编译对象**：写入语句在初始化时预编译了 `stmtInsert*`，但读取方法每次调用时都在内部重复执行 `db.prepare(...)`。

---

### 二、 [`server/ai.js`](../server/ai.js)（全长 221 行）—— 多模型 LLM 客户端适配器

#### 1. 角色定位与职责边界
属于 **Adapter / Infrastructure 层**。负责屏蔽不同大模型服务商（官方 Anthropic、DeepSeek、本地代理等）在环境变量加载、Base URL 后缀、鉴权 Header 及 Thinking 响应块结构上的差异。

#### 2. 核心设计拆解
* **零依赖 `.env` 加载（[`loadEnv`](../server/ai.js#L7-L43)）**：优先调用 Node 原生 `process.loadEnvFile()`，降级时手动解析，且严格遵守 `process.env[key] === undefined` 才写入，不覆盖命令行显式传入的环境变量。
* **Base URL 防重复拼接（[`normalizeBaseURL`](../server/ai.js#L51-L59)）**：自动剥离自定义端点末尾的 `/v1/messages` 或 `/v1`，防止 Anthropic SDK 二次拼接产生 `/v1/v1/messages`。
* **双模型分层配置（[`getConfig`](../server/ai.js#L117-L128)）**：
  * `chatModel`（默认 `claude-sonnet-4-20250514`）：负责复杂遥测分析与用户对话。
  * `agentModel`（默认 `claude-3-5-haiku-20241022`）：负责后台每 5 秒一次的低延迟、低成本自动化决策。
* **推理模型（Reasoning Models）文本提取兼容（[`extractText`](../server/ai.js#L190-L211)）**：遍历 `response.content` 数组优先提取 `type === 'text'` 的文本块，避免 DeepSeek R1 或 Claude 3.7 的前置 `thinking` 块导致解析失败。


---

## 第 3 站笔记：[`agent/run.sh`](../agent/run.sh)、[`agent/decide.js`](../agent/decide.js) 与策略文件

### 1. 角色定位与职责边界
`agent/` 目录构成了系统的 **自动化代理层（Autonomous Agent Layer）**。通过进程解耦，将“5 秒一次的 LLM 慢速推理”与“常驻的 `caretaker.js` 高频 WebSocket 通信”隔离开：
* **`agent/run.sh`（进程编排与心跳控制器，200 行）**：负责启动 `caretaker.js`、维护命名管道（FIFO）、执行 5 秒轮询、计算 Fear Backoff 状态并记录决策日志 `caretaker-decisions.log`。
* **`agent/decide.js`（无状态单次推理器，94 行）**：接收状态 Prompt，结合 `caretaker-policy.md` 调用轻量级大模型（`agentModel`），清洗并校验返回的 JSON 动作指令。
* **`caretaker-policy.md` & `chat-policy.md`（声明式策略层）**：分别定义了后台自动化控制的 7 条优先级铁律（强制 JSON 输出）与前台用户问答的诚实复盘准则（强制带数值的自然语言输出）。

### 2. 核心工程细节拆解
* **命名管道 `fd 3` 保活技巧（[`run.sh#L64-L70`](../agent/run.sh#L64-L70)）**：
  * 使用 `mkfifo "$FIFO"` 创建管道作为 `caretaker.js` 的 `stdin` 后，主脚本通过 `exec 3>"$FIFO"` 将文件描述符 `3` 持久绑定至管道写端。
  * 每次发送指令仅需 `echo "$compact" >&3`，避免了普通重定向在单次写入后关闭写端、触发 `caretaker.js` 收到 `EOF` 而意外退出的问题。
* **30 秒恐惧退避窗口（[`check_fear_backoff`](../agent/run.sh#L87-L100)）**：
  * 使用 `awk` 检测 `.drives.fear > 0.5`，一旦触发即更新 `FEAR_SPIKE_TIME`；在随后的 30 秒内强制将 `FEAR_BACKOFF: true` 注入 Prompt，阻止 Agent 对受惊果蝇继续施加动作。
* **跨平台毫秒级时间戳（[`run.sh#L149`](../agent/run.sh#L149)）**：
  * 针对 macOS BSD `date` 不支持 `%N`（纳秒/毫秒）的问题，使用 `python3 -c "import time; print(int(time.time()*1000))"` 生成 `CURRENT_TIME`，用于准确判定果蝇 `idle` 状态是否超过 120 秒。
* **双重防呆校验（[`decide.js#L25-L55`](../agent/decide.js#L25-L55) & [`run.sh#L180-L196`](../agent/run.sh#L180-L196)）**：
  * `decide.js` 内置正则剥离 Markdown ```` ```json ```` 围栏并执行 `JSON.parse()` 预校验；`run.sh` 外层二次校验 `.action` 是否属于 6 个合法动作白名单，确保非法 LLM 输出绝不进入命名管道。


---

## 第 4 站笔记：[`js/sim-worker.js`](../js/sim-worker.js) & [`js/brain-worker-bridge.js`](../js/brain-worker-bridge.js)

### 1. 角色定位与职责边界
这两个文件构成了前端的 **仿真引擎与适配器桥接层（Simulation Engine & Adapter Bridge）**：
* **`js/sim-worker.js`（后台计算引擎，486 行）**：运行于独立 Web Worker 线程，完全不知道“概念脑区名称”或“果蝇行为”，只对 `0 .. 139,254` 的连续整数下标执行 CSR 稀疏矩阵突触传播、LIF 电压衰减与 Neuropil 门控休眠。
* **`js/brain-worker-bridge.js`（双向适配器桥接层，652 行）**：站在主线程概念模型（`BRAIN`）与后台 Worker 之间：
  1. **下行翻译**：将 `BRAIN.stimulate` 和 `BRAIN.drives` 中的具名刺激（如 `'OLF_ORN_FOOD'`）展开为成千上万个具体的神经元下标数组发往 Worker；
  2. **上行翻译**：将 Worker 返回的各分组脉冲计数按脑区人数归一化，写回 `BRAIN.postSynaptic[name]`；
  3. **虚拟腹神经索合成（Virtual VNC）**：由于 FlyWire FAFB 仅包含头部大脑、不含胸腔腹神经索（腿/翅电机神经元在数据集中数量为 0），由 Bridge 读取下行神经元 `GNG_DESC` 与中央脑区活动，合成虚拟的六足与双翅电机输出。

### 2. 核心工程机制拆解
* **零拷贝初始化与无缝降级（[`initBridge`](../js/brain-worker-bridge.js#L91-L132) & [`handleWorkerMessage`](../js/brain-worker-bridge.js#L136-L217)）**：
  * 启动时先备份 `var legacyUpdate = BRAIN.update;`。
  * 下载 `connectome.bin.gz` 后通过 Transferable Objects `worker.postMessage({type: 'init', buffer}, [buffer])` 零拷贝移交内存所有权。
  * Worker 就绪后执行 `BRAIN.update = workerUpdate;` 接管主循环；若加载失败或 Worker 崩溃，自动恢复 `BRAIN.update = legacyUpdate;`（59 节点近似仿真）。
* **双通道刺激下发（[`sendOneShotStimuli`](../js/brain-worker-bridge.js#L496-L510) vs [`sendStimulation`](../js/brain-worker-bridge.js#L512-L547)）**：
  * **瞬发通道 (`stimulate`)**：针对单帧脉冲（如痛觉 `NOCI`），不等 Worker Tick 节拍，立即发消息直接注入 `V[idx]`，防止被后续状态刷新覆盖。
  * **持续通道 (`setStimulusState`)**：将当前所有激活的感觉与驱动力分组合并为扁平的 `allIndices` (`Uint32Array`) 与 `allIntensities` (`Float32Array`)，在每个 Worker Tick 的 Step 1.5 持续注入电流（`STIM_INTENSITY = 0.15`，配合 `0.95` 漏电率约 8 个 Tick 达到阈值放电）。
* **按脑区规模归一化与余晖保持（[`aggregateFireState`](../js/brain-worker-bridge.js#L551-L585)）**：
  * 计算公式：$\text{windowActivation} = \frac{\text{groupFires}[g]}{\text{groupSizes}[g] \times \text{tickWindow}} \times 100$，消除大脑区（如 8.2 万神经元的视髓 `VIS_ME`）与小脑区（如 34 神经元的 `SEZ_FEED`）的规模差异。
  * 采用 $\max(\text{windowActivation}, \text{prevActivation} \times 0.75)$ 平滑波谷抖动。
* **帧率解耦批处理（[`workerUpdate`](../js/brain-worker-bridge.js#L361-L412)）**：
  * 主线程 `BRAIN.update` 每 500ms（或按帧）调用，但仅在收到新 Worker Tick 数据时才执行完整的电机管线；期间累计的 `pendingDriveFrames` 会通过 `for` 循环批量补调 `BRAIN.updateDrives()`，确保指数衰减率（如 `fear *= 0.85`）严格符合过往帧数 $0.85^N$。


---

## 第 5 站笔记：[`js/connectome.js`](../js/connectome.js) & [`js/constants.js`](../js/constants.js)

### 1. 角色定位与职责边界
这两个文件构成了前端的 **核心领域模型层（Domain Model Layer）**：
* **`js/constants.js`（声明式拓扑图，501 行）**：定义了 59 个概念神经元群的有向带权邻接表 `weights`。按感觉（Sensory）、中央处理（MB / LH / CX / SEZ / GNG）、内驱力（Drives）和运动输出（DN / VNC / MN）四层组织。
* **`js/connectome.js`（`BRAIN` 领域对象与状态引擎，508 行）**：即使在 13.9 万神经元 Worker 启用后，它依然负责维护全局状态容器（`stimulate`、`drives`、`postSynaptic`）、执行每帧内驱力演化 `updateDrives()`，以及在末尾通过 `motorcontrol()` 将肌肉神经群电位清算为 7 个物理行为累加器（`accumWalkLeft/Right`, `accumFlight`, `accumFeed`, `accumGroom`, `accumStartle`, `accumHead`）。

### 2. 核心工程机制拆解
* **双缓冲状态切换（Double-Buffering，[`runconnectome`](../js/connectome.js#L394-L421)）**：
  * 每个节点拥有两个槽位 `postSynaptic[name] = [0, 0]`，由 `thisState` 与 `nextState` 索引。
  * 点火判定严格读取 `thisState`（阈值 `fireThreshold = 22`），突触电流累加与点火泄放（归零）严格写入 `nextState`，回合末尾统一同步并翻转指针。这保证了同一帧内所有神经元并发更新，消除了遍历顺序带来的级联偏差。
* **电机神经元“只收不发”拦截（Output-Only Sink，[`runconnectome#L396-L408`](../js/connectome.js#L396-L408)）**：
  * 凡是以 `MN_` 开头的电机神经元均被 `isMotor` 拦截，不进入 `fireNeuron()`。因为 `fireNeuron()` 会将节点在 `nextState` 的电位清零，拦截后可确保肌肉累积的兴奋量完整保留到紧随其后的 `motorcontrol()` 中读取。
* **读后即焚清算机制（Read-and-Drain，[`motorcontrol#L447-L452`](../js/connectome.js#L447-L452)）**：
  * `readMotor(name)` 读取 `MN_*` 在 `nextState` 的累积值后立即将其置 `0`，使肌肉输出严格反映当前时刻的瞬时运动指令，不产生跨周期的残余动作粘连。
  * 理毛累加器采用对称性检测：`accumGroom = abdomen + (abdomen > 0 ? head : 0) + Math.min(legL1, legR1)`，利用 `Math.min(legL1, legR1)` 区分“双前肢协同理毛”与“单侧腿偏置转弯”。


---

## 第 6 站笔记：[`js/fly-logic.js`](../js/fly-logic.js)、[`js/main.js`](../js/main.js) 与前端交互套件

### 1. 角色定位与职责边界
前端剩余的 12 个 JS 文件共同构成了 **表现与物理交互层（Presentation, Physics & UI Layer）**，按职责可划分为三个子模块：
1. **行为状态机纯函数层（[`js/fly-logic.js`](../js/fly-logic.js)，133 行）**：剥离了 DOM 依赖的纯逻辑模块（便于 Node.js 单元测试复用），负责根据 `BRAIN.accum*` 与 `BRAIN.drives` 执行 10 级优先级的行为状态仲裁（`evaluateBehaviorEntry`）。
2. **主控制器与 2D 物理画布（[`js/main.js`](../js/main.js)，2,089 行）**：管理 500ms 脑节拍与 60fps 渲染双时钟、处理鼠标/触摸屏手势、执行页面后台休眠状态保护（`visibilitychange`），并用 Canvas 2D 绘制果蝇身体各解剖部件与三足步态动画。
3. **可视化与 Caretaker 侧边栏套件**：
   * 脑可视化：[`js/neuro-renderer.js`](../js/neuro-renderer.js)（底部 13.9 万神经元 WebGL2 点阵）、[`js/brain3d.js`](../js/brain3d.js)（Three.js 3D 脑区解剖浮层）、[`js/education.js`](../js/education.js)（教学说明卡片）。
   * AI 饲养员前端套件：[`js/caretaker-bridge.js`](../js/caretaker-bridge.js)（连接 `ws://localhost:7600` 每秒上报状态并执行 6 种远端指令）、[`js/caretaker-sidebar.js`](../js/caretaker-sidebar.js)、[`js/caretaker-analytics.js`](../js/caretaker-analytics.js)、[`js/caretaker-calendar.js`](../js/caretaker-calendar.js)、[`js/caretaker-renderer.js`](../js/caretaker-renderer.js)。

### 2. 核心工程机制拆解
* **10 级行为优先级状态机（[`evaluateBehaviorEntry`](../js/fly-logic.js#L54-L92) & [`updateBehaviorState`](../js/main.js#L959-L1010)）**：
  * 严格按生存紧急度排序：`startle` (惊跳) > `fly` (飞行) > `feed` (进食) > `groom` (理毛) > `brace` (抗风抓地) > `rest` (睡眠) > `phototaxis` (趋光) > `explore` (好奇探索) > `walk` (行走) > `idle` (待机)。
  * 引入最短持续时间 `BEHAVIOR_MIN_DURATION` 与状态退出冷却表 `BEHAVIOR_COOLDOWN`，防止阈值边界附近的动作高频抖动；每次状态确认后通过 `syncBrainFlags()` 将 `_isMoving`、`_isFeeding`、`_isGrooming` 回写至 `BRAIN` 供下一轮内驱力结算。
* **双时钟协同架构（[`main.js`](../js/main.js)）**：
  * **500ms 慢时钟 (`updateBrain`)**：调用 `BRAIN.update()` $\rightarrow$ `updateBehaviorState()` $\rightarrow$ `computeMovementForBehavior()`（更新 `targetDir` 与 `targetSpeed`）$\rightarrow$ 刷新顶部 Drive 进度条。
  * **60fps 快时钟 (`loop` -> `update(dt)` + `draw()`)**：利用 `dtScale = dt / 16.667` 实现跨刷新率一致的速度插值、边界避障、食物距离检测（`<150px` 嗅觉激活，`<20px` 味觉接触与食物缩小）以及全身连杆绘制。
* **标签页后台休眠防漂移机制（[`visibilitychange`](../js/main.js#L685-L771)）**：
  * 解决浏览器在后台暂停 `requestAnimationFrame` 但继续运行 `setInterval` 导致内驱力无限累加爆表的问题。
  * 当 `document.hidden` 为真时，立即停止 `brainTickId` 与 `BRAIN.stopWorker()`，并保存 `driveSnapshotOnHide` 快照；切回前台时清空过期刺激、恢复内驱力快照、重置 `lastTime = -1`（防止巨型 `dt` 导致坐标穿墙），再重启 Worker 与定时器。


---

## 第七部分：端到端全链路追踪实战（End-to-End Trace）

模块拆解完成后，我们通过两个跨进程、跨线程的真实运行场景，把 **后端 Agent $\leftrightarrow$ WebSocket 网关 $\leftrightarrow$ 前端主线程 $\leftrightarrow$ Web Worker 13.9 万神经元** 串成一条完整的故事线。

### 场景一：饥饿累积 $\rightarrow$ AI 自动投喂 $\rightarrow$ 寻食与进食闭环

1. **内驱力累积（前端主线程，每 500ms）**：
   * [`BRAIN.updateDrives()`](../js/connectome.js#L171-L179) 每 tick 执行 `d.hunger += 0.005`。约 60 秒未进食后，`hunger` 突破 `0.6`。
   * 同时 [`js/caretaker-bridge.js`](../js/caretaker-bridge.js#L20-L23) 每 1 秒通过 WebSocket 将包含 `drives.hunger` 与 `food: []` 的状态快照推送给 [`server/caretaker.js`](../server/caretaker.js#L69-L85) 缓存至 `lastState`（若超过 `0.9` 仍无食物，[`detectIncidents()`](../server/caretaker.js#L52-L66) 会触发 `forgot_to_feed` 中危告警）。
2. **AI 饲养员感知与投喂决策（后端 Agent 进程，每 5s）**：
   * [`agent/run.sh`](../agent/run.sh#L140-L196) 轮询 `GET http://localhost:7600/state` 拿到状态，连同 `FEAR_BACKOFF` 和 `CURRENT_TIME` 传给 [`agent/decide.js`](../agent/decide.js)。
   * 大模型遵循 [`agent/caretaker-policy.md` 规则 4](../agent/caretaker-policy.md#L85-L90)（`hunger > 0.6` 且无食物），在果蝇上下左右随机偏移 `80px` 计算坐标，输出 `{"action":"place_food","params":{"x":...,"y":...}}`。
   * `run.sh` 校验动作合法后，通过持久文件描述符 `echo "$compact" >&3` 写入命名管道 FIFO $\rightarrow$ [`caretaker.js` `handleStdinCommand()`](../server/caretaker.js#L87-L115) 记录 `preFearLevel` 并落库 $\rightarrow$ WebSocket 下发 `command` $\rightarrow$ 前端 [`caretaker-bridge.js`](../js/caretaker-bridge.js#L36-L40) 向画布 `food` 数组推入一颗半径 `10px` 的食物。
3. **饥饿驱动搜索与嗅觉触发（Worker $\leftrightarrow$ Bridge $\leftrightarrow$ 状态机）**：
   * 食物刚落在 `80px` 处时，尚在 `50px` 嗅觉圈外（`foodNearby = false`）。但高企的 `d.hunger` 被 [`collectStimulationSegments()`](../js/brain-worker-bridge.js#L475-L478) 转化为 `DRIVE_HUNGER` 神经元群的持续电流，经 [`sim-worker.js`](../js/sim-worker.js) 突触传播激活 `MB_MBON_APP` 与 `CX_PFN`。
   * [`synthesizeMotorOutputs()`](../js/brain-worker-bridge.js#L296-L335) 合成 `walkDrive` 注入六条腿 `MN_LEG_*` $\rightarrow$ [`motorcontrol()`](../js/connectome.js#L455-L456) 累加 `accumWalkLeft/Right` $\rightarrow$ [`evaluateBehaviorEntry()`](../js/fly-logic.js#L84-L89) 进入 `'walk'` / `'explore'` 漫步。
   * 一旦漫步进入食物 **`50px` 范围内**：[`main.js` 距离检测](../js/main.js#L1940-L1942) 置 `BRAIN.stimulate.foodNearby = true`，向 `OLF_ORN_FOOD`（嗅觉感受器）注入电流；同时 [`computeMovementForBehavior()`](../js/main.js#L1046-L1055)（或切入 `'feed'` 后的趋近逻辑）用 `Math.atan2` 将果蝇朝向 `targetDir` 对准食物。
4. **接触刹停、伸喙进食与饱腹消退（60fps 物理帧 + 500ms 脑节拍）**：
   * 进入 `50px` 且满足 `accumFeed > threshold`（或 `hunger > 0.7 && foodNearby`）时，[`evaluateBehaviorEntry()`](../js/fly-logic.js#L64-L68) 切换状态为 **`'feed'`**，[`syncBrainFlags()`](../js/main.js#L997-L1009) 置 `BRAIN._isFeeding = true`。
   * 果蝇以 `0.25` 慢速滑行至 **`dist <= 20px`（接触圈）**：`foodContact = true` 激活味觉甜味神经元 `GUS_GRN_SWEET` $\rightarrow$ `SEZ_FEED` $\rightarrow$ `MN_PROBOSCIS`；物理层 `targetSpeed = 0` 刹停，口器动画 `anim.proboscisExtend` 伸出，食物半径在 `2~5` 秒内逐渐缩小直至 `food.splice(i, 1)` 吃完消失（[`main.js#L1944-L1958`](../js/main.js#L1944-L1958)）。
   * 进食期间每 500ms `d.hunger -= 0.3`（[`connectome.js#L176-L178`](../js/connectome.js#L176-L178)），饥饿值迅速归零，`DRIVE_HUNGER` 熄火，果蝇恢复平静 `'idle'`。

---

### 场景二：连续三击痛觉 $\rightarrow$ 惊跳逃飞 $\rightarrow$ 事故追责与 30s 恐惧退避

1. **三连击触发痛觉（前端画布交互）**：
   * 用户（或 Agent `touch` 指令）在果蝇 `50px` 内点击，[`applyTouchTool()`](../js/main.js#L893-L936) 将屏幕坐标逆旋转为果蝇局部坐标判定部位（头/胸/腹/腿），置 `BRAIN.stimulate.touch = true`，使每 tick `d.fear += 0.3`。
   * `touchTimestamps` 滑动窗口统计 **4 秒内满 3 次点击**时（[`main.js#L924-L935`](../js/main.js#L924-L935)），立即拉响最高警报：`BRAIN.stimulate.nociception = true`！
2. **瞬发通道直插 Worker 与逃生爆发（Bridge $\rightarrow$ Worker $\rightarrow$ 物理状态机）**：
   * 由于 `nociception` 仅维持单脑节拍，[`sendOneShotStimuli()`](../js/brain-worker-bridge.js#L496-L510) 走**瞬发通道**直接向 Worker 发送 `{ type: 'stimulate', indices: NOCI }`，瞬间抬高痛觉神经元电位。
   * 痛觉与 `DRIVE_FEAR` 洪流激活 `MB_MBON_AV`、`LH_AV` 与巨型纤维逃生通路 `DN_STARTLE`。[`synthesizeMotorOutputs()`](../js/brain-worker-bridge.js#L297-L346) 算出高额 `flightIntent`，同时向双翅 `MN_WING_L/R` 和 `DN_STARTLE` 注入强电流。
   * [`evaluateBehaviorEntry()`](../js/fly-logic.js#L58-L62) 命中第 1 优先级 **`'startle'`**！[`applyBehaviorMovement()`](../js/main.js#L1123-L1138) 执行经典昆虫两阶段逃逸：
     1. **`freeze`（僵直期）**：速度瞬间锁死为 `0`；
     2. **`burst`（弹射期）**：僵直结束瞬间赋予极速 `speed = 3.0`，朝反方向 `facingDir + Math.PI` 展翅弹射逃离，随后衔接 `'fly'` 高速飞行。
3. **后端事故抓拍与 30 秒退避保护（Gateway & Agent）**：
   * 若此次受惊发生在 Agent 执行动作后的 **5 秒追责窗口**内，且 `state.drives.fear - preFearLevel > 0.2`，[`detectIncidents()`](../server/caretaker.js#L42-L50) 当场判定 **`scared_the_fly`（`high` 级事故）**，写入 SQLite `incidents` 表（当日照料分扣 5 分）并通过 WebSocket 实时推送到前端侧边栏红字告警。
   * 下一轮 5 秒轮询中，[`agent/run.sh` 的 `check_fear_backoff()`](../agent/run.sh#L87-L100) 检测到 `drives.fear > 0.5`，立即记录 `FEAR_SPIKE_TIME`，在此后 **30 秒内**向 Prompt 强制注入 `FEAR_BACKOFF: true`。大模型命中 [`caretaker-policy.md` 第 1 铁律](../agent/caretaker-policy.md#L79)，无条件输出 `wait`，彻底停止一切打扰，等待果蝇 `fear` 按每帧 `*= 0.85` 自然平复。

---

## 第八部分：深度架构问答 —— 大脑神经计算与人工物理规则的真实边界

在精读 [`synthesizeMotorOutputs()`](../js/brain-worker-bridge.js#L273-L347)（人造腹神经索 Virtual VNC）与 [`js/main.js`](../js/main.js) 运动控制时，有三个最容易产生误解的核心架构问题，在此一并厘清：

### Q1：果蝇的行走方向和行走距离，是 13.9 万神经元大脑算出来的，还是人工算法算出来的？
* **“想不想走、油门踩多深”（标量速率 `walkDrive`）由大脑神经元算出**；
* **“往哪个方向转弯（`targetDir`）”与“离食物多远（`dist`）”由 [`main.js`](../js/main.js) 的人工几何算法（三角函数）算出**。
* **源码证据**：在 [`js/brain-worker-bridge.js#L321-L327`](../js/brain-worker-bridge.js#L321-L327) 中，代码注释明确写道：
  > `// Symmetric left/right walk output. Steering is handled by the behavioral layer (computeMovementForBehavior) using targetDir, not by leg asymmetry.`
  
  `walkL` 与 `walkR` 使用同一个标量 `walkDrive` 对称分配（仅附带 `±2%` 随机抖动防止走绝对直线）。真正的朝向偏转是在 [`js/main.js` 的 `computeMovementForBehavior()`](../js/main.js#L1046-L1055)（以及 `'feed'` 分支 [`L1091`](../js/main.js#L1091)）中通过 `Math.atan2(-(nf.item.y - fly.y), nf.item.x - fly.x)` 直接由二维坐标几何计算得出的。（关于如何用真实中央复合体 CX 神经元实现纯神经转向，详见配套专题文档 [`doc/cx-navigation-engineering.md`](./cx-navigation-engineering.md)）。

### Q2：果蝇是如何知道自己需要走多少步停下来的？
* 果蝇**没有任何路径规划或计步器**，而是依赖 **60fps 逐帧实时距离闭环检测（Reactive Closed-Loop）**，按距离阈值切换状态并踩刹车：
  1. **`dist > 50px`（无嗅觉漫步区）**：`foodNearby = false`，果蝇依靠 `DRIVE_HUNGER` 激发的 `walkDrive` 漫步搜索，步态相位 `anim.walkPhase` 随速度持续累加（[`main.js#L1193-L1196`](../js/main.js#L1193-L1196)）。
  2. **`20px < dist <= 50px`（嗅觉捕获与减速滑行区）**：进入 `50px` 瞬间，`foodNearby = true`（[`main.js#L1941-L1942`](../js/main.js#L1941-L1942)）且 `hasNearbyFood()` 返回 `true`（[`fly-logic.js#L41-L46`](../js/fly-logic.js#L41-L46)）。一旦切入 `'feed'` 状态，`anim.walkPhase` 停止推进（收腿停止迈步动画），物理目标速度被硬编码压低为 `targetSpeed = 0.25`（且最大速度钳制在 `0.2`，见 [`main.js#L1090-L1094`](../js/main.js#L1090-L1094) 与 [`L1151-L1155`](../js/main.js#L1151-L1155)），缓慢滑向食物防止冲过头（Overshoot）。
  3. **`dist <= 20px`（接触刹停区）**：触发 `foodContact = true`，`'feed'` 分支将 `targetSpeed` 置为 `0`，当前速度按每帧 `speed *= Math.pow(0.92, dtScale)` 指数衰减至彻底静止（[`main.js#L1095-L1098`](../js/main.js#L1095-L1098)、[`L1157-L1161`](../js/main.js#L1157-L1161)）。

### Q3：在靠近食物的过程中，`walkIntent` 是否会逐渐降低？
* **反直觉结论：不会！在靠近食物的路上，`walkIntent` 不仅不降低，反而保持高位（甚至微升）**。原因有三：
  1. **嗅觉输入是二值开关而非连续浓度梯度**：[`main.js#L1941-L1942`](../js/main.js#L1941-L1942) 中 `BRAIN.stimulate.foodNearby = (dist <= 50)` 是布尔值，[`collectStimulationSegments()`](../js/brain-worker-bridge.js#L457-L459) 对 `OLF_ORN_FOOD` 注入恒定的 `STIM_INTENSITY = 0.15`，不会随距离缩短而减弱。
  2. **接触进食前饥饿感仍在累加**：未真正进入进食消化前，`d.hunger` 每 tick 仍在 `+0.005`。
  3. **`walkIntent` 公式中气味趋近是纯正向权重，且未扣除进食抑制**：查看 [`js/brain-worker-bridge.js#L296`](../js/brain-worker-bridge.js#L296)：
     ```javascript
     var walkIntent = (cxPfn + cxFc + cxEpg) * 0.3 + (mbApp + lhApp) * 0.5 + (desc + vcpg) * 0.2;
     ```
     食物气味激活的 `mbApp`（蘑菇体趋近）和 `lhApp`（侧角趋近）占 `0.5` 正权重，直接推高 `walkIntent`；同时，不同于旧版 59 节点模型中 `SEZ_FEED -> DN_WALK (-6)` 的负向抑制（[`js/constants.js#L297`](../js/constants.js#L297)），13.9 万神经元的 `synthesizeMotorOutputs()` 并未从 `walkIntent` 中减去 `sezFeed`。
* **真正让它停下并降低 `walkIntent` 的机制**：先由 `main.js` 的 `'feed'` 状态分支在物理层强行接管速度（`targetSpeed = 0`），随后进食令 `d.hunger -= 0.3` 快速归零、食物吃完消失令 `foodNearby = false`，上游驱动源 `DRIVE_HUNGER` 与 `OLF_ORN_FOOD` 双双断电，`walkIntent` 才随之回落。
