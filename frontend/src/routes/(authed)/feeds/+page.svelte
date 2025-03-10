<script lang="ts">
	import PageHead from '$lib/components/PageHead.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as Tabs from '$lib/components/ui/tabs';
	import { AlertCircleIcon, CirclePauseIcon } from 'lucide-svelte';
	import moment from 'moment';
	import type { PageData } from './$types';
	import Actions from './Actions.svelte';
	import Detail from './Detail.svelte';

	interface Props {
		data: PageData;
	}

	let { data }: Props = $props();

	let showDetail = $state(false);
	let selectedGroup = $state(1);
	let selectedFeedID = $state(0);

	// ensure selectedFeed is reactive so that we can keep the info in detail
	// up-to-date
	let selectedFeed = $derived(
		data.groups.find((v) => v.id === selectedGroup)?.feeds.find((v) => v.id === selectedFeedID) ??
			data.groups[0].feeds[0]
	);

	function handleShowDetail(id: number) {
		showDetail = true;
		selectedFeedID = id;
	}
</script>

<svelte:head>
	<title>Feeds</title>
</svelte:head>

<PageHead title="Feeds" className="justify-between">
	<Actions groups={data.groups} />
</PageHead>

<Tabs.Root
	value={selectedGroup.toString()}
	onValueChange={(v) => v && (selectedGroup = parseInt(v))}
>
	<Tabs.List>
		{#each data.groups.sort((a, b) => a.id - b.id) as g}
			<Tabs.Trigger value={g.id.toString()}>
				{#if g.feeds.find((f) => f.failure && !f.suspended) !== undefined}
					<AlertCircleIcon size="15" class="fill-destructive text-destructive-foreground mr-1" />
				{/if}
				{g.name}
			</Tabs.Trigger>
		{/each}
	</Tabs.List>
	{#each data.groups as g}
		{@const gf = [
			...g.feeds.filter((v) => v.failure && !v.suspended),
			...g.feeds.filter((v) => !v.failure && !v.suspended),
			...g.feeds.filter((v) => v.suspended)
		]}
		<Tabs.Content value={g.id.toString()}>
			<ul>
				{#each gf as f}
					<li>
						<Button
							class="flex items-center w-full h-12 py-2 px-4 text-start gap-2"
							variant="ghost"
							onclick={() => handleShowDetail(f.id)}
						>
							<span class="w-[18px]">
								{#if f.suspended}
									<CirclePauseIcon class="w-[18px]" />
								{:else if f.failure}
									<AlertCircleIcon class="w-[18px] fill-destructive text-destructive-foreground" />
								{/if}
							</span>
							<span class="inline-block w-1/2 truncate">{f.name}</span>
							<span class="inline-block w-1/2 truncate text-muted-foreground">
								{#if f.failure}
									Error: {f.failure}
								{:else if f.suspended}
									Suspended
								{:else}
									<span class="hidden md:inline">Refreshed at </span>
									{moment(f.updated_at).format('LTS')}
								{/if}
							</span>
						</Button>
					</li>
				{/each}
			</ul>
		</Tabs.Content>
	{/each}
</Tabs.Root>

<Detail bind:show={showDetail} groups={data.groups} {selectedFeed} />
