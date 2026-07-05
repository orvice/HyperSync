import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { MediaUpload } from "@/components/media-upload";
import { postClient } from "@/lib/connect";

const VISIBILITY_OPTIONS = ["public", "unlisted", "private", "direct"] as const;
const PLATFORM_OPTIONS = ["mastodon", "bluesky", "threads", "memos"] as const;

export function CreatePostPage() {
  const navigate = useNavigate();
  const [content, setContent] = useState("");
  const [visibility, setVisibility] = useState<string>("public");
  const [syncTargets, setSyncTargets] = useState<string[]>([...PLATFORM_OPTIONS]);
  const [mediaItems, setMediaItems] = useState<{ id: string; cdnUrl: string; contentType: string }[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const toggleTarget = (target: string) => {
    setSyncTargets((prev) =>
      prev.includes(target) ? prev.filter((t) => t !== target) : [...prev, target]
    );
  };

  const handleSubmit = async (status: "draft" | "published") => {
    if (!content.trim()) {
      setError("Content is required");
      return;
    }
    setError("");
    setLoading(true);

    try {
      await postClient.createPost({
        content: content.trim(),
        visibility,
        status,
        syncTargets: status === "published" ? syncTargets : [],
        mediaIds: mediaItems.map((m) => m.id),
      });
      navigate("/");
    } catch {
      setError("Failed to create post");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen p-8 max-w-2xl mx-auto">
      <Card>
        <CardHeader>
          <CardTitle>New Post</CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="space-y-2">
            <Label htmlFor="content">Content</Label>
            <textarea
              id="content"
              className="flex min-h-[120px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              placeholder="What's on your mind?"
              value={content}
              onChange={(e) => setContent(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">{content.length} characters</p>
          </div>

          <div className="space-y-2">
            <Label>Media</Label>
            <MediaUpload value={mediaItems} onChange={setMediaItems} />
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
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}

          <div className="flex gap-3">
            <Button onClick={() => handleSubmit("published")} disabled={loading}>
              {loading ? "Publishing..." : "Publish"}
            </Button>
            <Button variant="outline" onClick={() => handleSubmit("draft")} disabled={loading}>
              Save Draft
            </Button>
            <Button variant="ghost" onClick={() => navigate("/")} disabled={loading}>
              Cancel
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
