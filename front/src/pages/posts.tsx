import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { useAuth } from "@/lib/auth";
import { postClient } from "@/lib/connect";
import type { Post } from "@/gen/api/v1/post_pb";

export function PostsPage() {
  const { logout } = useAuth();
  const [posts, setPosts] = useState<Post[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadPosts();
  }, []);

  const loadPosts = async () => {
    try {
      const resp = await postClient.listPosts({ pageSize: 20, page: 1 });
      setPosts(resp.posts);
      setTotal(resp.total);
    } finally {
      setLoading(false);
    }
  };

  const statusBadge = (status: string) => {
    const colors = status === "published"
      ? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
      : "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200";
    return <span className={`px-2 py-0.5 text-xs rounded-full ${colors}`}>{status}</span>;
  };

  const visibilityBadge = (visibility: string) => {
    return <span className="px-2 py-0.5 text-xs rounded-full bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300">{visibility}</span>;
  };

  return (
    <div className="min-h-screen p-8 max-w-4xl mx-auto">
      <div className="flex justify-between items-center mb-8">
        <h1 className="text-2xl font-bold">Posts</h1>
        <div className="flex gap-2">
          <Link to="/posts/new">
            <Button>New Post</Button>
          </Link>
          <Link to="/settings">
            <Button variant="ghost">Settings</Button>
          </Link>
          <Button variant="outline" onClick={logout}>Sign out</Button>
        </div>
      </div>

      {loading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : posts.length === 0 ? (
        <p className="text-muted-foreground">No posts yet. Create your first post!</p>
      ) : (
        <>
          <p className="text-sm text-muted-foreground mb-4">{total} posts</p>
          <div className="space-y-3">
            {posts.map((p) => (
              <Link key={p.id} to={`/posts/${p.id}`}>
                <Card className="hover:bg-accent/50 transition-colors cursor-pointer">
                  <CardContent className="py-4">
                    <div className="flex justify-between items-start">
                      <p className="text-sm line-clamp-2 flex-1 mr-4">{p.content}</p>
                      <div className="flex gap-2 shrink-0">
                        {visibilityBadge(p.visibility)}
                        {statusBadge(p.status)}
                      </div>
                    </div>
                    {p.syncTargets.length > 0 && (
                      <div className="mt-2 flex gap-1">
                        {p.syncTargets.map((t) => {
                          const status = p.crossPostStatus[t];
                          const bg = status?.success
                            ? "bg-green-50 text-green-700 dark:bg-green-900 dark:text-green-200"
                            : status?.error
                            ? "bg-red-50 text-red-700 dark:bg-red-900 dark:text-red-200"
                            : "bg-blue-50 text-blue-700 dark:bg-blue-900 dark:text-blue-200";
                          const icon = status?.success ? "✓" : status?.error ? "✗" : "⏳";
                          return (
                            <span key={t} className={`px-1.5 py-0.5 text-xs rounded ${bg}`}>
                              {icon} {t}
                            </span>
                          );
                        })}
                      </div>
                    )}
                  </CardContent>
                </Card>
              </Link>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
