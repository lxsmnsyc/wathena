import { defineConfig } from 'oxfmt';

export default defineConfig({
  singleQuote: true,
  // Markdown follows the repository's own style.
  ignorePatterns: ['dist', 'data', 'src/**/testdata', 'src/gen', '**/*.md'],
});
