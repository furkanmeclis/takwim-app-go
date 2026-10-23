# Activity history (TEC-893)

`/t/{slug}/activity` lists who changed what in the business, when and in which branch. The same API serves the future mobile app.

## API

- `GET /v1/tenant/activity` (`tenant.activity.read`): page of events, newest first. Filters: `q` (action, actor, branch name), `resource` and `action` (comma-separated), `resource_uuid` (one record's history), `actor_uuid`, `from` / `to` (RFC 3339), `sort=created_at|-created_at`.
- `GET /v1/tenant/activity/facets`: resources and actions with counts, for the faceted filters.
- Each event carries its actor (null for system events), its branch (null for business-wide events) and `payload`. Updates store the before / after of every changed field in `payload.changes` (`{"name": {"before": "A", "after": "B"}}`).

`tenant.activity.read` is granted to `owner`, `admin` and `sube_yoneticisi` (migration 000043).

## Branch scope

`activity_events.branch_id` (migration 000043) is set from `activity.Entry.BranchUUID` when the event belongs to one branch; existing branch events were backfilled. A member limited to some branches, or with a branch picked in the header (`X-Branch`), only sees events of those branches. Business-wide events (settings, roles, members, profile) are only shown with all branches in view and full branch access.

## Recording changes

`activity.Diff(before, after, skip...)` compares two values by their JSON form and returns the changed fields as dot paths (nested objects down to three levels, arrays as a whole). Handlers read the record before the update and pass the result as `Entry.Changes`:

| Action                 | Compared                                                   |
| ---------------------- | ---------------------------------------------------------- |
| `branches.updated`     | the branch (timestamps, images and derived fields skipped) |
| `settings.updated`     | previous and saved settings document                       |
| `members.updated`      | roles, branch scope, branch names                          |
| `organization.updated` | name, time zone, currency                                  |

New modules record the same way: build an `activity.Entry` with `BranchUUID` for branch-bound records and `Changes` for updates, add the action and resource labels to `activity.json` (tr and en) and, when needed, field labels as `activity.fields.{resource}.{path}`.
