
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import type { DomainRecord, EntityConfig, PriorityDecisionRevision } from '../types/domain';
import { allowedTargets, formatDate } from '../utils/format';
import { useAuth } from '../hooks/useAuth';
import StatusBadge from './common/StatusBadge.vue';
import SeverityBadge from './common/SeverityBadge.vue';
import EvidenceGallery from './common/EvidenceGallery.vue';
import MetricCard from './common/MetricCard.vue';
import ConfirmDialog from './common/ConfirmDialog.vue';

const props = withDefaults(defineProps<{ config: EntityConfig; store: any; showEvidence?: boolean }>(), { showEvidence: false });
const { session, canAtLeast } = useAuth();
const search = ref('');
const showCreate = ref(false);
const pending = ref<{ item: DomainRecord; status: string } | null>(null);
const canWrite = computed(() => canAtLeast('operator'));
const highRisk = computed(() => props.store.items.filter((item: DomainRecord) => ['high', 'critical'].includes(item.riskLevel)).length);
const restrictedCount = computed(() => props.store.items.filter((item: DomainRecord) => item.status === 'restricted').length);
const unconfirmedCount = computed(() => props.store.items.reduce((sum: number, item: DomainRecord) => sum + (item.unconfirmedDefectCount || 0), 0));
const transitionHint = computed(() => {
	if (!pending.value) return '';
	if (props.config.key === 'priorityDecision') {
		return pending.value.status === 'observe'
			? 'observe 仅进入监测，不改变桥梁状态；定稿后不可覆盖。'
			: '定稿为限速/立即处置后，同设施桥梁将同事务进入受限；桥梁已关闭或停用时定稿会被拒绝。';
	}
	if (props.config.key === 'bridgeAsset' && pending.value.item.status === 'restricted' && pending.value.status === 'active') {
		const remaining = pending.value.item.unconfirmedDefectCount || 0;
		return remaining > 0
			? `仍有 ${remaining} 条同设施缺陷处于 new/verified 确认阶段，恢复运行请求会被拒绝。`
			: '确认阶段缺陷已清零，复核人可将桥梁改回正常运行。';
	}
	return '状态迁移会写入审计日志并保留请求号。';
});

onMounted(() => void props.store.load(props.config.path));

function targetsFor(item: DomainRecord): readonly string[] {
	if (props.config.key === 'priorityDecision') {
		if (!canAtLeast('reviewer') || item.preparedBy === session.value?.username) return [];
	} else if (props.config.key === 'bridgeAsset') {
		// 受限桥梁恢复运行只能由复核人/admin 发起；其余桥梁迁移维持 operator。
		if (!canWrite.value) return [];
		return allowedTargets(props.config.key, item.status).filter(target =>
			!(item.status === 'restricted' && target === 'active') || canAtLeast('reviewer'));
	} else if (!canWrite.value) return [];
	return allowedTargets(props.config.key, item.status);
}

function latestRevision(item: DomainRecord): PriorityDecisionRevision | undefined {
	return item.revisions?.[item.revisions.length - 1];
}

async function createDemo() {
	const now = Date.now();
	const created = await props.store.createRecord(props.config.path, {
		code: `${props.config.key.toUpperCase()}-${String(now).slice(-6)}`, name: `新增${props.config.label}`,
		description: '通过前端工作台创建的业务记录', facility: 'K42 桥梁作业区', owner: session.value?.displayName || '现场操作员',
		category: '结构复核', riskLevel: 'medium', metricValue: 25, metricUnit: 'score', effectiveAt: new Date().toISOString(),
		evidence: '现场照片、量测记录与检查批次已完成核对', relatedCode: props.config.key === 'priorityDecision' ? 'DF-001' : 'IR-001',
	});
	if (created) { search.value = ''; showCreate.value = false; }
}

async function confirmTransition() {
	if (!pending.value) return;
	const changed = await props.store.transition(props.config.path, pending.value.item, pending.value.status);
	if (changed) { search.value = ''; pending.value = null; }
}
</script>

<template>
	<main class="workspace">
		<header class="page-header">
			<div><p class="eyebrow">业务工作台</p><h1>{{ config.label }}</h1><p>统一管理{{ config.label }}的状态、风险、证据与责任人。</p></div>
			<el-button v-if="canWrite" type="primary" @click="showCreate = true">新增{{ config.label }}</el-button>
		</header>
		<section class="metrics">
			<MetricCard label="记录总数" :value="store.meta.total" detail="当前筛选范围"/>
			<MetricCard label="高风险" :value="highRisk" detail="需要优先复核"/>
			<MetricCard v-if="config.key === 'bridgeAsset'" label="限速受限桥梁" :value="restrictedCount" detail="定稿联动受限，调度按限速放行"/>
			<MetricCard v-if="config.key === 'bridgeAsset'" label="未确认缺陷" :value="unconfirmedCount" detail="同设施 new/verified 未清零，不能恢复运行"/>
			<MetricCard label="状态种类" :value="new Set(store.items.map((item: DomainRecord) => item.status)).size" detail="状态机覆盖"/>
		</section>
		<section v-if="showEvidence" class="evidence-panel"><header><strong>证据摘要</strong><span>最近四条记录</span></header><EvidenceGallery :records="store.items"/></section>
		<section class="toolbar"><el-input v-model="search" :placeholder="`搜索${config.label}编码或名称`" clearable/><el-button type="primary" @click="store.load(config.path, search)">查询</el-button><el-button @click="search = ''; store.load(config.path)">重置</el-button></section>
		<el-alert v-if="store.error" :title="store.error" type="error" show-icon/>
		<section class="table-shell">
			<el-table v-loading="store.loading" :data="store.items">
				<el-table-column prop="code" label="编码" width="150"/>
				<el-table-column label="名称" min-width="180"><template #default="{ row }"><strong>{{ row.name }}</strong><small>{{ row.facility }}</small></template></el-table-column>
				<el-table-column label="状态" width="130"><template #default="{ row }"><StatusBadge :status="row.status"/></template></el-table-column>
				<el-table-column label="风险" width="90"><template #default="{ row }"><SeverityBadge v-if="['defectFinding', 'priorityDecision'].includes(config.key)" :severity="row.riskLevel"/><span v-else>{{ row.riskLevel }}</span></template></el-table-column>
				<el-table-column v-if="config.key === 'bridgeAsset'" label="限速 / 未确认缺陷" width="180"><template #default="{ row }">
					<div class="bridge-restriction">
						<el-tag v-if="row.status === 'restricted'" type="warning" effect="dark" size="small">限速受限</el-tag>
						<el-tag v-else-if="row.status === 'closed' || row.status === 'retired'" type="danger" size="small">已{{ row.status === 'closed' ? '关闭' : '停用' }}·不放行</el-tag>
						<el-tag v-else type="success" size="small">正常运行</el-tag>
						<small :class="{ 'remaining-block': (row.unconfirmedDefectCount || 0) > 0 }">未确认缺陷 {{ row.unconfirmedDefectCount || 0 }} 条</small>
					</div>
				</template></el-table-column>
				<el-table-column prop="owner" label="责任人" min-width="130"/>
				<el-table-column label="指标" width="120"><template #default="{ row }">{{ row.metricValue }} {{ row.metricUnit }}</template></el-table-column>
				<el-table-column v-if="config.key === 'priorityDecision'" label="版本审计" width="250"><template #default="{ row }"><strong>v{{ row.version }} · {{ row.preparedBy }}</strong><small>{{ latestRevision(row)?.actor }} · {{ latestRevision(row)?.requestId }}</small><small>{{ latestRevision(row)?.evidence }}</small></template></el-table-column>
				<el-table-column label="更新时间" width="180"><template #default="{ row }">{{ formatDate(row.updatedAt) }}</template></el-table-column>
				<el-table-column label="操作" width="300"><template #default="{ row }"><div class="row-actions"><el-button v-for="target in targetsFor(row)" :key="target" link type="primary" @click="pending = { item: row, status: target }">推进至 {{ target }}</el-button><span v-if="targetsFor(row).length === 0" class="muted">无可用操作</span></div></template></el-table-column>
			</el-table>
		</section>
		<ConfirmDialog v-model="showCreate" :title="`新增${config.label}`" @confirm="createDemo"><p>将创建一条包含完整责任人、风险和证据信息的记录。</p></ConfirmDialog>
		<ConfirmDialog :model-value="Boolean(pending)" title="确认状态迁移" @update:model-value="pending = null" @confirm="confirmTransition"><p>{{ transitionHint }}</p><strong>{{ pending?.item.status }} → {{ pending?.status }}</strong></ConfirmDialog>
	</main>
</template>
