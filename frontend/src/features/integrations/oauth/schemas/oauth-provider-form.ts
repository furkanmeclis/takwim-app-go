import { z } from "zod";

export const oauthProviderFormSchema = z.object({
  login_enabled: z.boolean(),
  register_enabled: z.boolean(),
  client_id: z.string().trim(),
  client_secret: z.string().trim().optional(),
  // Apple signing key (.p8); validated by the API on save.
  team_id: z.string().trim().optional(),
  key_id: z.string().trim().optional(),
  /** PEM text of the chosen .p8 file (never pre-filled from the API). */
  private_key: z.string().optional(),
  remove_private_key: z.boolean().optional(),
});

export type OAuthProviderFormValues = z.infer<typeof oauthProviderFormSchema>;
