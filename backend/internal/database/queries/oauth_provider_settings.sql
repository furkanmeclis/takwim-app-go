-- name: GetOAuthProviderSettings :one
SELECT * FROM oauth_provider_settings
WHERE provider = sqlc.arg(provider);

-- name: ListOAuthProviderSettings :many
SELECT * FROM oauth_provider_settings
ORDER BY provider ASC;

-- name: UpdateOAuthProviderSettings :one
UPDATE oauth_provider_settings
SET login_enabled = COALESCE(sqlc.narg(login_enabled), login_enabled),
    register_enabled = COALESCE(sqlc.narg(register_enabled), register_enabled),
    client_id = COALESCE(sqlc.narg(client_id), client_id),
    client_secret_enc = COALESCE(sqlc.narg(client_secret_enc), client_secret_enc),
    apple_team_id = COALESCE(sqlc.narg(apple_team_id), apple_team_id),
    apple_key_id = COALESCE(sqlc.narg(apple_key_id), apple_key_id),
    apple_private_key_enc = CASE
        WHEN sqlc.arg(clear_apple_private_key)::boolean THEN NULL
        ELSE COALESCE(sqlc.narg(apple_private_key_enc), apple_private_key_enc)
    END
WHERE provider = sqlc.arg(provider)
RETURNING *;
