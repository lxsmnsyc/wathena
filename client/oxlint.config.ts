import config from '@lxsmnsyc/oxlint-config';
import { defineConfig } from 'oxlint';

export default defineConfig({
  extends: [config],
  ignorePatterns: ['dist'],
  overrides: [
    {
      // Tests read fixtures and inspect three.js objects, whose types are
      // loose.
      files: ['**/*.test.ts'],
      rules: {
        'typescript/explicit-function-return-type': 'off',
        'typescript/no-unsafe-type-assertion': 'off',
        'typescript/no-unsafe-call': 'off',
        'typescript/no-unsafe-member-access': 'off',
      },
    },
  ],
});
