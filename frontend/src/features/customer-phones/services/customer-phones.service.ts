import type { ServerListParams } from "@/components/entity";
import type {
  CustomerPhoneReassignmentInput,
  CustomerPhoneRelease,
  CustomerPhoneReleasePage,
} from "@/features/customer-phones/types";
import { platformRequest } from "@/lib/api/platform-request";

export type ListCustomerPhoneReleasesParams = ServerListParams & {
  /** Comma separated reasons. */
  reason?: string;
  /** Comma separated sources. */
  source?: string;
};

const PATH = "/v1/platform/customer-phone-releases";

export const customerPhonesService = {
  list(params: ListCustomerPhoneReleasesParams) {
    return platformRequest<CustomerPhoneReleasePage>("GET", PATH, {
      query: {
        limit: params.limit,
        offset: params.offset,
        sort: params.sort,
        q: params.q,
        reason: params.reason,
        source: params.source,
      },
    });
  },

  /** Processes a reassignment notice; the caller renders the errors. */
  reassign(body: CustomerPhoneReassignmentInput) {
    return platformRequest<CustomerPhoneRelease>("POST", PATH, {
      body,
      silent: true,
    });
  },
};
