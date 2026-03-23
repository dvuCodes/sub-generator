import { useCallback, useState } from "react";
import { cn } from "@/lib/utils";
import { HugeiconsIcon } from "@hugeicons/react";
import { Film01Icon, CloudUploadIcon } from "@hugeicons/core-free-icons";

interface VideoDropzoneProps {
  selectedFile: string | null;
  onFileSelect: (path: string) => void;
  disabled?: boolean;
}

const SUPPORTED_EXTENSIONS = [
  ".mp4",
  ".mkv",
  ".avi",
  ".mov",
  ".webm",
  ".flv",
  ".wmv",
  ".m4v",
];

export function VideoDropzone({
  selectedFile,
  onFileSelect,
  disabled,
}: VideoDropzoneProps) {
  const [isDragOver, setIsDragOver] = useState(false);

  const handleBrowse = useCallback(async () => {
    const { open: openDialog } = await import("@tauri-apps/plugin-dialog");
    const selected = await openDialog({
      multiple: false,
      filters: [
        {
          name: "Video Files",
          extensions: SUPPORTED_EXTENSIONS.map((extension) => extension.slice(1)),
        },
      ],
    });

    if (selected) {
      onFileSelect(selected as string);
    }
  }, [onFileSelect]);

  const handleDragOver = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      if (!disabled) {
        setIsDragOver(true);
      }
    },
    [disabled]
  );

  const handleDragLeave = useCallback(() => {
    setIsDragOver(false);
  }, []);

  const handleDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      setIsDragOver(false);
      if (disabled) {
        return;
      }

      const file = event.dataTransfer.files[0];
      if (!file) {
        return;
      }

      const extension = "." + file.name.split(".").pop()?.toLowerCase();
      if (!SUPPORTED_EXTENSIONS.includes(extension)) {
        return;
      }

      const droppedPath = (file as File & { path?: string }).path;
      if (droppedPath) {
        onFileSelect(droppedPath);
      }
    },
    [disabled, onFileSelect]
  );

  const fileName = selectedFile?.split(/[/\\]/).pop() ?? null;

  return (
    <button
      type="button"
      className={cn(
        "group relative w-full border border-dashed p-8 text-center transition-all",
        "focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring/50 outline-none",
        isDragOver && "border-primary bg-primary/5",
        !isDragOver && !selectedFile && "border-border hover:border-muted-foreground hover:bg-muted/30",
        selectedFile && "border-chart-1/40 bg-chart-1/5",
        disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer"
      )}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      onClick={disabled ? undefined : handleBrowse}
      disabled={disabled}
    >
      {selectedFile ? (
        <div className="flex flex-col items-center gap-3">
          <div className="flex size-12 items-center justify-center border border-chart-1/30 bg-chart-1/10">
            <HugeiconsIcon icon={Film01Icon} className="size-6 text-chart-1" strokeWidth={1.5} />
          </div>
          <div className="space-y-1">
            <p className="text-xs font-medium uppercase tracking-[0.2em] text-chart-1">
              Selected
            </p>
            <p className="font-medium text-foreground">{fileName}</p>
            <p className="text-xs text-muted-foreground">
              Click to choose a different file
            </p>
          </div>
        </div>
      ) : (
        <div className="flex flex-col items-center gap-3">
          <div className="flex size-12 items-center justify-center border border-border bg-muted/50 transition-colors group-hover:border-muted-foreground/50 group-hover:bg-muted">
            <HugeiconsIcon icon={CloudUploadIcon} className="size-6 text-muted-foreground transition-colors group-hover:text-foreground" strokeWidth={1.5} />
          </div>
          <div className="space-y-1">
            <p className="text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
              Input Video
            </p>
            <p className="font-medium text-foreground">
              Click to select a video file
            </p>
            <p className="text-xs text-muted-foreground">
              {SUPPORTED_EXTENSIONS.join(" ")}
            </p>
          </div>
        </div>
      )}
    </button>
  );
}
