
<script setup lang="ts">import { computed } from 'vue'; import { statusTone } from '../../utils/format'; import { BRIDGE_STATUS_LABELS } from '../../types/status';
const props = defineProps<{ status: string; labels?: Readonly<Record<string, string>> }>();
const tone = computed(() => statusTone(props.status));
const text = computed(() => props.labels?.[props.status] ?? BRIDGE_STATUS_LABELS[props.status] ?? props.status.replaceAll('_', ' '));</script>
<template><span :class="`status status--${tone}`">{{ text }}</span></template>
