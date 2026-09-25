import type { EntityConfig } from './domain';

export type DefectState = 'new' | 'verified' | 'monitoring' | 'mitigated' | 'closed';
export const ALL_DEFECT_STATE: readonly DefectState[] = ['new', 'verified', 'monitoring', 'mitigated', 'closed'];
export type PriorityLevel = 'observe' | 'restrict' | 'urgent';
export const ALL_PRIORITY_LEVEL: readonly PriorityLevel[] = ['observe', 'restrict', 'urgent'];

// 缺陷仍处于确认阶段（new/verified）。同设施存在这类缺陷时，受限桥梁不能恢复正常运行。
export const DEFECT_CONFIRMATION_STATES: readonly DefectState[] = ['new', 'verified'];

// 桥梁限速/停用相关状态的中文展示。
export const BRIDGE_STATUS_LABELS: Readonly<Record<string, string>> = {
  active: '正常运行',
  restricted: '限速受限',
  closed: '已关闭',
  retired: '已停用',
};

export const ENTITY_CONFIGS: readonly EntityConfig[] = [
  { key: 'bridgeAsset', path: 'bridges', label: '桥梁资产', statuses: ['active', 'restricted', 'closed', 'retired'] as const },
  { key: 'inspectionRound', path: 'inspections', label: '检查批次', statuses: ['planned', 'running', 'review', 'completed'] as const },
  { key: 'defectFinding', path: 'defects', label: '缺陷发现', statuses: ['new', 'verified', 'monitoring', 'mitigated', 'closed'] as const },
  { key: 'priorityDecision', path: 'priorities', label: '优先级决定', statuses: ['draft', 'observe', 'restrict', 'urgent'] as const }
];
