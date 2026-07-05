import { useEffect, useState } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { postClient } from "@/lib/connect";
import type { Post } from "@/gen/api/v1/post_pb";

export function PostDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [post, setPost] = useState<Post | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!id) return;
    loadPost();
  }, [id]);

  const loadPost = async () => {
    setError("");
    try {
      const resp = await postClient.getPost({ id: id! });
      setPost(resp.post!);
    } catch {
      setError("Failed to load post. It may not exist or the server is unavailable.");
    } finally {
      setLoading(false);
    }
  };

  const handlePublish = async () => {
    if (!post) return;
    setError("");
    try {
      const resp = await postClient.publishPost({ id: post.id });
      setPost(resp.post!);
    } catch {
      setError("Failed to publish post. Please try again.");
    }
  };

  const handleDelete = async () => {
    if (!post) return;

    const syncedPlatforms = Object.entries(post.crossPostStatus)
      .filter(([, s]) => s.success)
      .map(([p]) => p);

    const msg = syncedPlatforms.length > 0
      ? `This will also delete from: ${syncedPlatforms.join(", ")}.\n\nAre you sure?`
      : "Delete this post?";

    if (!confirm(msg)) return;
    setError("");
    try {
      await postClient.deletePost({ id: post.id });
      navigate("/");
    } catch {
      setError("Failed to delete post. Please try again.");
    }
  };

  if (loading) return <p className="p-8 text-muted-foreground">Loading...</p>;
  if (!post) {
    return (
      <div className="min-h-screen p-8 max-w-2xl mx-auto">
        <Button variant="ghost" onClick={() => navigate("/")} className="mb-4">&larr; Back</Button>
        <p className="text-sm text-destructive">{error || "Post not found."}</p>
      </div>
    );
  }

  return (
    <div className="min-h-screen p-8 max-w-2xl mx-auto">
      <Button variant="ghost" onClick={() => navigate("/")} className="mb-4">&larr; Back</Button>

      <Card>
        <CardHeader>
          <div className="flex justify-between items-center">
            <CardTitle className="text-lg">Post Detail</CardTitle>
            <div className="flex gap-2">
              <span className={`px-2 py-0.5 text-xs rounded-full ${post.status === "published" ? "bg-green-100 text-green-800" : "bg-yellow-100 text-yellow-800"}`}>
                {post.status}
              </span>
              <span className="px-2 py-0.5 text-xs rounded-full bg-gray-100 text-gray-600">
                {post.visibility}
              </span>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="whitespace-pre-wrap text-sm">{post.content}</div>

          {post.syncTargets.length > 0 && (
            <div>
              <p className="text-xs text-muted-foreground mb-2">Sync targets</p>
              <div className="flex gap-1 flex-wrap">
                {post.syncTargets.map((t) => (
                  <span key={t} className="px-2 py-0.5 text-xs bg-blue-50 text-blue-700 rounded dark:bg-blue-900 dark:text-blue-200">{t}</span>
                ))}
              </div>
            </div>
          )}

          {Object.keys(post.crossPostStatus).length > 0 && (
            <div>
              <p className="text-xs text-muted-foreground mb-2">Sync status</p>
              <div className="space-y-1">
                {Object.entries(post.crossPostStatus).map(([platform, status]) => (
                  <div key={platform} className="flex items-center gap-2 text-xs">
                    <span className="font-medium w-20">{platform}</span>
                    <span className={status.success ? "text-green-600" : "text-red-600"}>
                      {status.success ? "synced" : status.error || "pending"}
                    </span>
                    {status.retryCount > 0 && (
                      <span className="text-muted-foreground">(retries: {status.retryCount})</span>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          {error && <p className="text-sm text-destructive">{error}</p>}

          <div className="flex gap-3 pt-4 border-t">
            <Link to={`/posts/${post.id}/edit`}>
              <Button variant="outline">Edit</Button>
            </Link>
            {post.status === "draft" && (
              <Button onClick={handlePublish}>Publish</Button>
            )}
            <Button variant="destructive" onClick={handleDelete}>Delete</Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
