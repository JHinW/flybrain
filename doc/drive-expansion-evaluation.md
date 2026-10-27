# 内驱力（Drives）扩展评估与底层连接组数据诊断报告

本文档基于对 [`data/classification.csv.gz`](../data/classification.csv.gz)、[`data/neuron_meta.json`](../data/neuron_meta.json) 及构建脚本 [`scripts/build_connectome.py`](../scripts/build_connectome.py) 的底层核查，评估在工程上逐步补齐、完善和新增果蝇内驱力（Drives）的可行性与实施路线。

---

## 1. 现状诊断：为什么当前有 28 个神经元群数量为 0？

对 [`data/neuron_meta.json`](../data/neuron_meta.json) 的统计显示，在 59 个具名功能群中，有 **28 个群的真实神经元数量为 `0`**，同时有 **21,955 个神经元（占比 15.8%）** 落入了未分类的 `GENERIC_CENTRAL`。

经排查 [`scripts/build_connectome.py`](../scripts/build_connectome.py) 中的 `determine_group()` 逻辑，发现三类主要原因：

### ① 映射规则 Bug（本地数据已有明确标注，但被脚本写错分支漏检）
* **生物钟神经元（`CLOCK_DN`）漏检 193 个神经元**：
  * 在 `classification.csv.gz` 中，果蝇的背侧生物钟神经元被标注为 `super_class="central", class="", sub_class="DN1p"`（28 个）和 `sub_class="DN3"`（165 个）。
  * 但 [`scripts/build_connectome.py#L298-L300`](../scripts/build_connectome.py#L298-L300) 将 `"dn1p"` 和 `"dn3"` 错误地写在了 `if "descend" in super_class:`（下行神经元）分支下（误把 Dorsal Neurons 当作 Descending Neurons）。导致这 193 个生物钟神经元全部落入 `GENERIC_CENTRAL`，而 `CLOCK_DN` 的神经元数量变成了 `0`。

### ② 单一归并（未利用神经递质 `nt_type` 或细分亚类拆分正负通路）
* **奖惩与趋避通路合并**：
  * 脚本将全部 335 个 `class="dan"` 归入 `MB_DAN_REW`，全部 96 个 `class="mbon"` 归入 `MB_MBON_APP`，全部 559 个 `lhln/lhcent` 归入 `LH_APP`。
  * 结果导致对应的回避/惩罚通路 `MB_DAN_PUN`、`MB_MBON_AV`、`LH_AV` 神经元数量全部为 `0`。实际上结合 `neurons.csv.gz` 中的 `nt_type`（如兴奋性 `ACH`/`GLUT` 与抑制性 `GABA`）及投射关系即可实现正负通路拆分。

### ③ 缺少细粒度 `cell_type` 列（导致高级中央/下行神经元无法按具体型号匹配）
* 当前 `data/classification.csv.gz` 包含 8 列：`['root_id', 'flow', 'super_class', 'class', 'sub_class', 'hemilineage', 'side', 'nerve']`。
* 其中：
  * `super_class="central", class="", sub_class=""` 有 **21,762** 个神经元；
  * `super_class="descending", class="", sub_class=""` 有 **1,303** 个神经元（全部落入 `GNG_DESC`，导致 `DN_WALK`、`DN_FLIGHT`、`DN_TURN`、`DN_BACKUP`、`DN_STARTLE` 数量全为 `0`）。
* 要精确识别其中的 `MB_APL`（蘑菇体全局抑制）、`pC1`/`P1`（求偶驱动）、`aIPg`（攻击驱动）、`oviDN`（产卵驱动）以及具体的下行指令神经元（如 `DNp01`、`DNa02`），需要引入 FlyWire FAFB v783 官方公开的 `consolidated_cell_types.csv.gz`（按 `root_id` 关联 `primary_type`）。

---

## 2. 现有本地数据对完善/新增 Drive 的直接支撑能力

在不下载任何外部数据的前提下，`data/` 目录中已有的神经元标注足以支撑以下内驱力的修复与新增：

| 内驱力 / 模块 | 本地数据支撑情况 (`classification.csv.gz`) | 工程可行性 |
| :--- | :--- | :--- |
| **昼夜节律与睡眠稳态 (`CLOCK_DN` & `DRIVE_FATIGUE`)** | 已有 **193 个生物钟神经元**（`DN1p`: 28, `DN3`: 165）+ **34 个脑侧部内分泌细胞**（`pars_lateralis / lateral_NSC`） | ⭐⭐⭐⭐⭐ 修正 `build_connectome.py` 分支即可立刻激活 |
| **口渴与饮水稳态 (`DRIVE_THIRST` & `SEZ_WATER`)** | 已有 **131 个水味觉神经元**（`GUS_GRN_WATER`）+ **6 个咽下区神经分泌细胞**（`endocrine / SEZ-NSC`），且 `constants.js` 已内置 `SEZ_WATER` 连接权重 | ⭐⭐⭐⭐⭐ 零外部数据依赖，补充 `drives.thirst` 状态与水源交互即可闭环 |
| **体温调节驱力 (`DRIVE_THERMO`)** | 已有 **49 个热敏神经元**（`THERMO_WARM`）+ **54 个冷敏神经元**（`THERMO_COOL`）及完整全脑突触连接 | ⭐⭐⭐⭐⭐ 可直接基于现有温度状态建立趋温内稳态 |
| **饥饿与代谢内稳态 (`DRIVE_HUNGER`)** | 已有 **40 个脑间部内分泌细胞**（`pars_intercerebralis / medial_NSC`，果蝇胰岛素产生细胞 IPCs 所在地） | ⭐⭐⭐⭐⭐ 已激活，可进一步与 `SEZ-NSC` 区分糖/水需求 |

---

## 3. 分阶段工程实施路线图（Roadmap）

### Phase 1：修复现有 `build_connectome.py` 映射缺陷（零外部数据依赖）
1. **解救 `CLOCK_DN`**：将 `sub_class in ("dn1p", "dn3")` 的判定移至 `super_class == "central"` 分支前，恢复 193 个真实生物钟神经元。
2. **细分内分泌细胞（`endocrine`）**：
   * `pars_intercerebralis` (40) $\rightarrow$ `DRIVE_HUNGER`
   * `pars_lateralis` (34) $\rightarrow$ `DRIVE_FATIGUE`
   * `SEZ-NSC` (6) $\rightarrow$ `SEZ_WATER`（或新增的 `DRIVE_THIRST`）
3. **拆分趋近与回避通路**：结合 `neurons.csv.gz` 的神经递质 `nt_type`（`GABA` 抑制性 vs `ACH`/`GLUT` 兴奋性）或突触上下游投射，将 `MBON`、`DAN`、`LH` 拆分为 `APP/REW` 与 `AV/PUN` 两组，消除空群。

### Phase 2：在现有数据上新增 `DRIVE_THIRST`（口渴）与 `DRIVE_THERMO`（温控）
1. 在 `BRAIN.drives` 中新增 `thirst` 与 `thermoDiscomfort` 状态变量及演化方程。
2. 激活 `constants.js` 中已定义但处于休眠状态的 `GUS_GRN_WATER` $\rightarrow$ `SEZ_WATER` $\rightarrow$ `MN_PROBOSCIS` 饮水回路。
3. 在前端工具栏支持水滴放置或温区梯度，形成完整的“口渴寻水”与“避暑/避寒”行为闭环。

### Phase 3：引入 FlyWire `cell_types` 注释表，解锁高级社交与繁殖 Drive
1. 下载 FlyWire FAFB v783 官方 `consolidated_cell_types.csv.gz` 并按 `root_id` 合并至构建管线。
2. 精确映射 21,762 个中央神经元与 1,303 个下行神经元：
   * 补齐 `MB_APL`、`SEZ_GROOM` 及 `DN_WALK` / `DN_FLIGHT` / `DN_TURN` / `DN_BACKUP` / `DN_STARTLE` 的真实神经元集合。
   * 解锁高级内驱力节点：**求偶驱力（`DRIVE_COURTSHIP`：`pC1`/`P1`）**、**攻击驱力（`DRIVE_AGGRESSION`：`aIPg`）**、**产卵驱力（`DRIVE_OVIPOSITION`：`oviDN`）**。
