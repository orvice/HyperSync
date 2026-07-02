import { useEffect, useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { postClient } from "@/lib/connect";

const VISIBILITY_OPTIONS = ["public", "unlisted", "private", "direct"] as const;
const PLATFORM_OPTIONS = ["mastodon", "bluesky", "threads", "memos"] as const;

export function EditPostPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [content, setContent] = useState("");
  const [visibility, setVisibility] = useState<string>("public");
  const [syncTargets, setSyncTargets] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!id) return;
    loadPost();
  }, [id]);

  const loadPost = async () => {
    try {
      const resp = await postClient.getPost({ id: id! });
      const post = resp.post!;
      setContent(post.content);
      setVisibility(post.visibility);
      setSyncTargets(post.syncTargets);
    } catch {
      navigate("/");
    } finally {
      setLoading(false);
    }
  };

  const toggleTarget = (target: string) => {
    setSyncTargets((prev) =>
      prev.includes(target) ? prev.filter((t) => t !== target) : [...prev, target]
    );
  };

  const handleSave = async () => {
    if (!content.trim()) {
      setError("Content is required");
      return;
    }
    setError("");
    setSaving(true);

    try {
      await postClient.updatePost({
        id: id!,
        content: content.trim(),
        visibility,
        syncTargets,
      });
      navigate(`/posts/${id}`);
    } catch {
      setError("Failed to update post");
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <p className="p-8 text-muted-foreground">Loading...</p>;

  return (
    <div className="min-h-screen p-8 max-w-2xl mx-auto">
      <Button variant="ghost" onClick={() => navigate(`/posts/${id}`)} className="mb-4">&larr; Back</Button>

      <Card>
        <CardHeader>
          <CardTitle>Edit Post</CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="space-y-2">
            <Label htmlFor="content">Content</Label>
            <textarea
              id="content"
              className="flex min-h-[120px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              value={content}
              onChange={(e) => setContent(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">{content.length} characters</p>
          </div>

          <div className="space-y-2">
            <Label>Visibility</Label>
            <div className="flex gap-2 flex-wrap">
              {VISIBILITY_OPTIONS.map((v) => (
                <Button
                  key={v}
                  type="button"
                  variant={visibility === v ? "default" : "outline"}
                  size="sm"
                  onClick={() => setVisibility(v)}
                >
                  {v}
                </Button>
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <Label>Sync to</Label>
            <div className="flex gap-2 flex-wrap">
              {PLATFORM_OPTIONS.map((p) => (
                <Button
                  key={p}
                  type="button"
                  variant={syncTargets.includes(p) ? "default" : "outline"}
                  size="sm"
                  onClick={() => toggleTarget(p)}
                >
                  {p}
                </Button>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">
              Changing content will trigger re-sync to already-synced platforms.
            </p>
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}

          <div className="flex gap-3">
            <Button onClick={handleSave} disabled={saving}>
              {saving ? "Saving..." : "Save Changes"}
            </Button>
            <Button variant="ghost" onClick={() => navigate(`/posts/${id}`)} disabled={saving}>
              Cancel
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
