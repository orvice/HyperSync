import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { AuthService } from "@/gen/api/v1/auth_pb";
import { PostService } from "@/gen/api/v1/post_pb";

const transport = createConnectTransport({
  baseUrl: "/",
  interceptors: [
    (next) => async (req) => {
      const token = localStorage.getItem("token");
      if (token) {
        req.header.set("Authorization", `Bearer ${token}`);
      }
      return next(req);
    },
  ],
});

export const authClient = createClient(AuthService, transport);
export const postClient = createClient(PostService, transport);
