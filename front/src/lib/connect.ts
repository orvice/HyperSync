import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { AuthService } from "@/gen/api/v1/auth_pb";
import { PostService } from "@/gen/api/v1/post_pb";

function handleUnauthenticated() {
  localStorage.removeItem("token");
  localStorage.removeItem("token_expires_at");
  if (window.location.pathname !== "/login") {
    window.location.href = "/login";
  }
}

const transport = createConnectTransport({
  baseUrl: "/",
  interceptors: [
    (next) => async (req) => {
      const token = localStorage.getItem("token");
      if (token) {
        req.header.set("Authorization", `Bearer ${token}`);
      }
      try {
        return await next(req);
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
          handleUnauthenticated();
        }
        throw err;
      }
    },
  ],
});

export const authClient = createClient(AuthService, transport);
export const postClient = createClient(PostService, transport);
