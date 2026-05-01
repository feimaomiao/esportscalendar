import js from '@eslint/js';
import globals from 'globals';
import prettierConfig from 'eslint-config-prettier/flat';

// Globals our own JS defines and consumes across files. Marked `writable` so
// the file that defines them isn't flagged by `no-redeclare`.
const projectGlobals = {
	htmx: 'readonly',
	initGameSelection: 'writable',
	checkAndUpdateSubmitButton: 'writable',
	showToast: 'writable',
	setButtonLoading: 'writable',
	executeScriptsIn: 'writable',
	convertMatchTimesIn: 'writable',
};

export default [
	{
		ignores: [
			'static/js/vendor/**',
			'static/css/**',
			'node_modules/**',
			'components/*_templ.go',
			'tmp/**',
			'esportscalendar',
		],
	},
	js.configs.recommended,
	{
		files: ['static/js/**/*.js'],
		languageOptions: {
			ecmaVersion: 'latest',
			sourceType: 'script',
			globals: {
				...globals.browser,
				...projectGlobals,
			},
		},
		rules: {
			// Empty catch blocks are deliberate guards around browser APIs
			// (sessionStorage in private mode, clipboard without focus, etc.).
			'no-empty': ['error', { allowEmptyCatch: true }],
			// `} catch (e) {` patterns where `e` is intentionally ignored.
			// `vars: 'local'` skips top-level function/var declarations so
			// cross-file globals (initGameSelection, checkAndUpdateSubmitButton,
			// etc.) loaded via separate <script> tags don't get flagged as
			// "unused" in their defining file.
			'no-unused-vars': [
				'error',
				{ vars: 'local', args: 'after-used', argsIgnorePattern: '^_', caughtErrors: 'none' },
			],
			// Built-in JS redeclarations stay errors; our cross-file globals
			// (writable in projectGlobals) are exempt.
			'no-redeclare': ['error', { builtinGlobals: false }],
		},
	},
	// Disables eslint formatting rules that would conflict with Prettier.
	// Must be last so it overrides earlier configs.
	prettierConfig,
];
