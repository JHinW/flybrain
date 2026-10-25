# 中央复合体（Central Complex）导航机制与神经转向工程方案

## 1. 现状分析：当前代码如何计算下一个位置？

在当前的 FlyBrain 实现中，中央复合体（`CX_*`）与底层的 2D 物理移动采用了**“神经驱动标量速度 + 行为层几何转向”**的混合架构：

1. **神经层（计算标量油门）**：
   * 在 [`js/brain-worker-bridge.js`](../js/brain-worker-bridge.js) 的 `synthesizeMotorOutputs()` 中，系统汇总 `CX_PFN`、`CX_FC`、`CX_EPG` 的放电量计算出总行走意图 `walkIntent`，并对称地分配给左右六足（`MN_LEG_L1..L3` 与 `MN_LEG_R1..R3`）。
2. **行为层（计算目标角度 `targetDir`）**：
   * 在 [`js/main.js`](../js/main.js) 的 `computeMovementForBehavior()` 中，为了防止 Web Worker 的随机脉冲噪声导致果蝇原地疯狂打转（`motor asymmetry capped to prevent worker noise from causing spinning`），左右腿的不对称转向被限制在 `[-0.05, 0.05]`。
   * 真正的转向角度 `targetDir` 由外部几何计算（如对食物、光源、屏幕边界做 `Math.atan2`）决定。
3. **物理渲染层（60fps 更新坐标）**：
   * 每一帧将当前朝向 `facingDir` 平滑插值逼近 `targetDir`，再用三角函数更新位置：
     ```javascript
     fly.x += Math.cos(facingDir) * speed * dtScale;
     fly.y -= Math.sin(facingDir) * speed * dtScale;
     ```

---

## 2. 为什么纯靠当前脉冲差值会导致“原地打转”？

1. **输入端缺乏空间左右不对称性**：气味（`OLF_ATTR`）与光照（`VIS_PHOTO`）目前作为标量注入整个功能群，左半脑与右半脑接收到的刺激完全相同。
2. **输出端空间拓扑被压缩为标量**：13.9 万神经元的脉冲按功能群求和后，`CX_EPG` 丢失了环状吸引子（Ring Attractor）的扇区角度信息；左右腿电机神经元的瞬时差值主要由高频泊松脉冲噪声（Poisson spike noise）主导。

---

## 3. 解决随机打转、实现纯神经转向的三大工程方案

若要在工程上移除 `main.js` 中的 `Math.atan2` 硬编码，完全依靠神经网络涌现出稳定的转向与导航，可采用以下三种由浅入深的工程方案：

### 方案 1：双侧差分传感 + 低通滤波（Braitenberg 车辆模型）

通过模拟双侧感官（左右触角、左右复眼）的物理浓度差，配合时间平滑滤波消除高频脉冲噪声。

* **实现步骤**：
  1. **输入拆分左右**：在 [`js/brain-worker-bridge.js`](../js/brain-worker-bridge.js) 注入刺激时，根据目标方位与果蝇当前朝向的夹角 $\Delta\theta = \theta_{\text{target}} - \theta_{\text{facing}}$，分别计算左右两侧的刺激强度：
     * 左侧感觉输入 $\propto 1 + \sin(\Delta\theta)$
     * 右侧感觉输入 $\propto 1 - \sin(\Delta\theta)$
  2. **下行通道拆分左右**：将转向与运动神经元按解剖学中线（$x < \text{midline}$ 与 $x > \text{midline}$）拆分为左右两组（如 `DN_TURN_L` / `DN_TURN_R`）。
  3. **指数移动平均（EMA 低通滤波）**：
     随机脉冲噪声属于高频抖动，而转向意图属于低频持续信号。对左右脉冲差值施加一阶低通滤波：
     $$\text{turnRate}_{t} = \alpha \cdot \text{turnRate}_{t-1} + (1 - \alpha) \cdot (\text{Spike}_{R} - \text{Spike}_{L}), \quad \alpha \in [0.85, 0.95]$$
     高频随机噪声在时间窗口内正负抵消，输出平滑稳定的转向角速度。

### 方案 2：群体向量解码（Population Vector Decoding，还原 360° 生物指南针）

利用真实果蝇椭球体（Ellipsoid Body, EB）中 `CX_EPG` 神经元的环状空间拓扑，通过群体投票消除单点噪声。

* **实现步骤**：
  1. **扇区角度绑定**：在预处理 FlyWire 连接组数据时，根据 `CX_EPG`（约 50 个神经元）在椭球体中的 3D 解剖坐标，将其划分为 8 或 16 个扇区，为第 $i$ 个神经元分配偏好角度 $\phi_i \in [0, 2\pi)$。
  2. **复数相位 / 群体向量重心解码**：
     每一帧不再将所有 `CX_EPG` 脉冲简单相加，而是计算所有神经元放电率 $r_i$ 的向量重心：
     $$\theta_{\text{brain}} = \operatorname{atan2}\left(\sum_{i} r_i \sin\phi_i,\; \sum_{i} r_i \cos\phi_i\right)$$
  3. **抗噪优势**：即使个别神经元出现随机自发放电，50 个神经元构成的环状吸引子（Ring Attractor）能量包重心依然极其稳定，可直接映射为果蝇的内部目标朝向。

### 方案 3：闭环视动反射负反馈（Sensorimotor Optomotor Damping）

在真实果蝇与神经机器人（Neuro-robotics）中，航向稳定依赖于高频率的“感觉-运动闭环”阻尼。

* **实现步骤**：
  1. **引入角速度光流反馈**：当随机脉冲噪声导致左腿多迈一步、身体意外向右偏转角速度 $\omega$ 时，背景图像相对复眼向左滑动。
  2. **激活全视野光流神经元（`VIS_LPTC`）**：将身体旋转角速度 $-\omega$ 实时转化为左右不对称的 `VIS_LPTC` 视觉刺激输入。
  3. **闭环纠偏**：`VIS_LPTC` 投射至中央复合体与下行转向神经元（`DN_TURN`），产生与意外偏转方向相反的纠偏力矩（类似控制工程中的微分阻尼项 $D$）。
  4. **工程前提**：需要较高的仿真器同步频率（如 50~100Hz），以保证闭环反馈延迟小于身体偏转的时间常数。

---

## 4. 三大方案对比总结

| 工程方案 | 改动复杂度 | 生物学还原度 | 抗噪与防打转核心机制 |
| :--- | :---: | :---: | :--- |
| **方案 1：双侧差分 + EMA 低通滤波** | ⭐ 低 | ⭐⭐⭐ | 高频泊松脉冲噪声在时间窗口内正负抵消，由双侧感官浓度差驱动平滑转向 |
| **方案 2：EPG 群体向量解码** | ⭐⭐ 中 | ⭐⭐⭐⭐⭐ | 环状吸引子群体空间投票求向量重心，天然免疫单神经元随机闪烁 |
| **方案 3：视动反射闭环阻尼** | ⭐⭐⭐ 高 | ⭐⭐⭐⭐ | 意外偏转实时触发反向光流刺激，通过闭环负反馈产生抗旋转阻尼 |
