import { sveltekit } from '@sveltejs/kit/vite';
import adapter from '@sveltejs/adapter-auto';
import { defineConfig } from 'vite';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit({
			adapter: adapter(),
			// Kit 3 removed $lib; upstream AI Elements components all import from it.
			alias: { '$lib': 'src/lib' }
		})]
});
