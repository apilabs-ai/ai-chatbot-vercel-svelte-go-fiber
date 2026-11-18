<script lang="ts">
	import {
		PromptInput,
		PromptInputActionAddAttachments,
		PromptInputActionMenu,
		PromptInputActionMenuContent,
		PromptInputActionMenuTrigger,
		PromptInputAttachment,
		PromptInputAttachments,
		PromptInputBody,
		PromptInputButton,
		type PromptInputMessage,
		PromptInputSubmit,
		PromptInputTextarea,
		PromptInputToolbar,
		PromptInputTools,
	} from "$lib/components/ai-elements/prompt-input/index.js";
	import * as ModelSelector from "$lib/components/ai-elements/model-selector/index.js";
	import { Check, ChevronDown, Globe, ImagePlus, Brain } from "@lucide/svelte";
	import type { ModelInfo } from "$lib/chat/api";

	interface ChatInputProps {
		placeholder?: string;
		input: string;
		status: "idle" | "submitted" | "streaming" | "error";
		onSubmit: (message: PromptInputMessage, event?: SubmitEvent) => void;
		/** Allow-listed models from the backend; selector hidden when empty */
		models?: ModelInfo[];
		model?: string;
		webSearch?: boolean;
		imageGen?: boolean;
	}

	let {
		placeholder = "Send a message...",
		input = $bindable(),
		status,
		onSubmit,
		models = [],
		model = $bindable(""),
		webSearch = $bindable(false),
		imageGen = $bindable(false),
	}: ChatInputProps = $props();

	let modelOpen = $state(false);
</script>

<div class="mx-auto w-full max-w-3xl px-4 pb-4 pt-2 shrink-0">
	<PromptInput globalDrop multiple onSubmit={onSubmit}>
		<PromptInputBody>
			<PromptInputAttachments>
				{#snippet children(attachment)}
					<PromptInputAttachment data={attachment} />
				{/snippet}
			</PromptInputAttachments>
			<PromptInputTextarea
				{placeholder}
				bind:value={input}
				onchange={(e) => (input = (e.target as HTMLTextAreaElement).value)}
			/>
		</PromptInputBody>
		<PromptInputToolbar>
			<PromptInputTools>
				<PromptInputActionMenu>
					<PromptInputActionMenuTrigger />
					<PromptInputActionMenuContent>
						<PromptInputActionAddAttachments />
					</PromptInputActionMenuContent>
				</PromptInputActionMenu>
				<PromptInputButton
					size="sm"
					variant={webSearch ? "secondary" : "ghost"}
					aria-pressed={webSearch}
					onclick={() => (webSearch = !webSearch)}
				>
					<Globe class="size-4" />
					<span>Search</span>
				</PromptInputButton>
				<PromptInputButton
					size="sm"
					variant={imageGen ? "secondary" : "ghost"}
					aria-pressed={imageGen}
					onclick={() => (imageGen = !imageGen)}
				>
					<ImagePlus class="size-4" />
					<span>Image</span>
				</PromptInputButton>
				{#if models.length > 0}
					<ModelSelector.Root bind:open={modelOpen}>
						<ModelSelector.Trigger
							class="inline-flex h-8 items-center gap-1.5 rounded-lg px-2 text-sm text-muted-foreground hover:bg-accent hover:text-foreground"
						>
							<ModelSelector.Logo provider="openai" />
							<ModelSelector.Name>{model}</ModelSelector.Name>
							<ChevronDown class="size-3.5 opacity-50" />
						</ModelSelector.Trigger>
						<ModelSelector.Content>
							<ModelSelector.Input placeholder="Search models..." />
							<ModelSelector.List>
								<ModelSelector.Empty>No models found.</ModelSelector.Empty>
								<ModelSelector.Group heading="OpenAI">
									{#each models as m (m.id)}
										<ModelSelector.Item
											value={m.id}
											onSelect={() => {
												model = m.id;
												modelOpen = false;
											}}
										>
											<ModelSelector.Logo provider="openai" />
											<ModelSelector.Name>{m.id}</ModelSelector.Name>
											{#if m.reasoning}
												<Brain class="size-3.5 text-muted-foreground" aria-label="Reasoning model" />
											{/if}
											{#if model === m.id}
												<Check class="ml-auto size-4" />
											{:else}
												<div class="ml-auto size-4"></div>
											{/if}
										</ModelSelector.Item>
									{/each}
								</ModelSelector.Group>
							</ModelSelector.List>
						</ModelSelector.Content>
					</ModelSelector.Root>
				{/if}
			</PromptInputTools>
			<PromptInputSubmit {status} />
		</PromptInputToolbar>
	</PromptInput>
	<p class="mt-2 text-center text-[10px] text-muted-foreground">
		AI responses may be inaccurate. Verify important information.
	</p>
</div>
