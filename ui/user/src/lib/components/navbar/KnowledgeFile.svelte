<script lang="ts">
	import { CircleX, FileText, Trash } from 'lucide-svelte/icons';
	import { type KnowledgeFile } from '$lib/services';
	import { popover } from '$lib/actions';
	import Loading from '$lib/icons/Loading.svelte';

	interface Props {
		onDelete?: () => void;
		file: KnowledgeFile;
	}

	const { onDelete, file }: Props = $props();

	const tt = popover({
		placement: 'top-start',
		hover: true
	});
</script>

<div class="group flex" use:tt.ref>
	<button class="flex flex-1 items-center">
		<FileText class="h-5 w-5" />
		<span class="ms-3"
			>{file.fileName.length > 26 ? file.fileName.slice(0, 26) + '...' : file.fileName}</span
		>
		{#if file.state === 'error' || file.state === 'failed'}
			<CircleX class="ms-2 h-4 text-red-600" />
		{:else if file.state === 'pending' || file.state === 'ingesting'}
			<Loading class="mx-1.5" />
		{/if}

		{@render ttContent()}
	</button>
	<button
		class="hidden group-hover:block"
		onclick={() => {
			if (file.state === 'ingested') {
				onDelete?.();
			}
		}}
	>
		<Trash class="h-5 w-5 text-gray" />
	</button>
</div>

{#snippet ttContent()}
	{#if file.state === 'error' || file.state === 'failed'}
		<p class="rounded-xl bg-red-600 p-2" use:tt.tooltip>
			{file.error ? file.error : 'Failed'}
		</p>
	{:else}
		<p class="rounded-xl bg-blue-500 p-2 text-white dark:text-black" use:tt.tooltip>
			{file.fileName}
		</p>
	{/if}
{/snippet}
