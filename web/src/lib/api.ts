import createClient from "openapi-fetch";
import type { paths } from "./api.gen";

export const api = createClient<paths>({
  baseUrl: "/api/v1",
  credentials: "include",
  querySerializer: {
    array: {
      style: "form",
      explode: true,
    },
  },
});
