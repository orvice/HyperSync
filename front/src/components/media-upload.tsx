import { useState, useRef } from "react";
import { Button } from "@/components/ui/button";
import { uploadMedia } from "@/lib/media";

interface MediaItem {
  id: string;
  cdnUrl: string;
  contentType: string;
}

interface MediaUploadProps {
  value: MediaItem[];
  onChange: (items: MediaItem[]) => void;
}

export function MediaUpload({ value, onChange }: MediaUploadProps) {
  const [uploading, setUploading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const handleFiles = async (files: FileList | null) => {
    if (!files || files.length === 0) return;
    setUploading(true);

    try {
      const newItems: MediaItem[] = [];
      for (const file of Array.from(files)) {
        const result = await uploadMedia(file);
        newItems.push({
          id: result.id,
          cdnUrl: result.cdn_url,
          contentType: result.content_type,
        });
      }
      onChange([...value, ...newItems]);
    } finally {
      setUploading(false);
      if (inputRef.current) inputRef.current.value = "";
    }
  };

  const remove = (id: string) => {
    onChange(value.filter((m) => m.id !== id));
  };

  return (
    <div className="space-y-3">
      {value.length > 0 && (
        <div className="flex gap-2 flex-wrap">
          {value.map((m) => (
            <div key={m.id} className="relative group">
              {m.contentType.startsWith("image/") ? (
                <img src={m.cdnUrl} alt="" className="w-20 h-20 object-cover rounded border" />
              ) : (
                <div className="w-20 h-20 flex items-center justify-center rounded border bg-muted text-xs">
                  {m.contentType.split("/")[1]}
                </div>
              )}
              <button
                type="button"
                onClick={() => remove(m.id)}
                className="absolute -top-1 -right-1 w-5 h-5 bg-destructive text-destructive-foreground rounded-full text-xs opacity-0 group-hover:opacity-100 transition-opacity"
              >
                &times;
              </button>
            </div>
          ))}
        </div>
      )}

      <input
        ref={inputRef}
        type="file"
        multiple
        accept="image/*,video/*,.pdf"
        className="hidden"
        onChange={(e) => handleFiles(e.target.files)}
      />
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => inputRef.current?.click()}
        disabled={uploading}
      >
        {uploading ? "Uploading..." : "Add Media"}
      </Button>
    </div>
  );
}
