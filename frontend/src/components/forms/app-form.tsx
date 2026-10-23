"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import type { ReactNode } from "react";
import {
  FormProvider,
  useForm,
  type DefaultValues,
  type FieldValues,
  type SubmitHandler,
  type UseFormReturn,
} from "react-hook-form";
import type { ZodType } from "zod";

import { ApiError } from "@/lib/api";

type AppFormProps<T extends FieldValues> = {
  schema: ZodType<T>;
  defaultValues?: DefaultValues<T>;
  onSubmit: SubmitHandler<T>;
  children: ReactNode | ((form: UseFormReturn<T>) => ReactNode);
  className?: string;
  id?: string;
};

export function AppForm<T extends FieldValues>({
  schema,
  defaultValues,
  onSubmit,
  children,
  className,
  id,
}: AppFormProps<T>) {
  const form = useForm<T>({
    // zod v4 + RHF resolver typing can be stricter than runtime needs
    resolver: zodResolver(schema as never),
    defaultValues,
  });

  const handleSubmit: SubmitHandler<T> = async (values) => {
    try {
      await onSubmit(values);
    } catch (error) {
      // Validation errors and conflicts (e.g. a duplicate code) may carry
      // field details; show them under the matching inputs.
      if (
        error instanceof ApiError &&
        (error.isValidation || error.status === 409)
      ) {
        const fields = error.fieldErrors();
        Object.entries(fields).forEach(([field, message]) => {
          form.setError(field as never, { type: "server", message });
        });
      }
      throw error;
    }
  };

  return (
    <FormProvider {...form}>
      <form
        id={id}
        className={className}
        onSubmit={form.handleSubmit(handleSubmit)}
        noValidate
      >
        {typeof children === "function" ? children(form) : children}
      </form>
    </FormProvider>
  );
}
