import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import eslintConfigPrettier from "eslint-config-prettier";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  eslintConfigPrettier,
  {
    files: ["src/**/*.{ts,tsx}"],
    ignores: ["src/components/ui/date-picker.tsx"],
    rules: {
      "@typescript-eslint/no-unused-vars": [
        "warn",
        {
          argsIgnorePattern: "^_",
          varsIgnorePattern: "^_",
          caughtErrorsIgnorePattern: "^_",
          ignoreRestSiblings: true,
        },
      ],
      "no-restricted-syntax": [
        "error",
        {
          selector: "JSXAttribute[name.name='type'][value.value='date']",
          message:
            'Use DatePicker from @/components/ui/date-picker (or AppDatePicker in forms) instead of native type="date".',
        },
      ],
    },
  },
  // UI rules (AGENTS.md): screens build every control from the shadcn
  // primitives in components/ui and every list from the DataTable stack.
  // The block repeats the date rule because a later no-restricted-syntax
  // replaces the earlier one for these files.
  {
    files: ["src/features/**/*.tsx", "src/app/**/*.tsx"],
    ignores: ["src/app/global-error.tsx"],
    rules: {
      "no-restricted-syntax": [
        "error",
        {
          selector: "JSXAttribute[name.name='type'][value.value='date']",
          message:
            'Use DatePicker from @/components/ui/date-picker (or AppDatePicker in forms) instead of native type="date".',
        },
        {
          selector:
            "JSXOpeningElement[name.name=/^(table|thead|tbody|tr|td|th)$/]",
          message:
            "Lists use the DataTable stack (EntityTable / DataTable from @/components/entity, @/components/tables), not a native <table>.",
        },
        {
          selector: "JSXOpeningElement[name.name=/^(select|textarea)$/]",
          message:
            "Use the shadcn primitives in @/components/ui (or the App* form fields in @/components/forms), not native form controls.",
        },
        {
          // A hidden file input behind a drop zone is the one native input.
          selector:
            "JSXOpeningElement[name.name='input']:not(:has(JSXAttribute[name.name='type'][value.value=/^(file|hidden)$/]))",
          message:
            "Use Input from @/components/ui (or the App* form fields in @/components/forms), not a native <input>.",
        },
      ],
    },
  },
  // Editor X (Lexical / shadcn-editor) uses ref-during-render and effect
  // patterns that conflict with React Compiler eslint rules.
  {
    files: ["src/components/editor/**/*.{ts,tsx}"],
    rules: {
      "react-hooks/refs": "off",
      "react-hooks/set-state-in-effect": "off",
      "react-hooks/use-memo": "off",
      "react/display-name": "off",
      "@next/next/no-img-element": "off",
      "@typescript-eslint/no-empty-object-type": "off",
      "@typescript-eslint/ban-ts-comment": "off",
    },
  },
  globalIgnores([
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
    "src/generated/**",
  ]),
]);

export default eslintConfig;
