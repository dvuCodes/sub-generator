import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { HugeiconsIcon } from "@hugeicons/react";
import {
  CheckmarkCircle02Icon,
  FolderOpenIcon,
  RefreshIcon,
} from "@hugeicons/core-free-icons";
import { deriveOutputDirectory, explorerOpenTarget } from "@/lib/outputPath";

interface OutputResultProps {
  outputPath: string;
  transcriptionLog?: string;
  segments: number;
  durationSecs: number;
  onReset: () => void;
}

export function OutputResult({
  outputPath,
  transcriptionLog,
  segments,
  durationSecs,
  onReset,
}: OutputResultProps) {
  const fileName = outputPath.split(/[/\\]/).pop() ?? outputPath;
  const dir = deriveOutputDirectory(outputPath);

  const openInExplorer = async () => {
    try {
      const { open } = await import("@tauri-apps/plugin-shell");
      await open(explorerOpenTarget(outputPath));
    } catch (err) {
      console.error("Failed to open directory:", err);
    }
  };

  const formatDuration = (secs: number) =>
    secs < 60
      ? `${Math.round(secs)}s`
      : `${Math.floor(secs / 60)}m ${Math.round(secs % 60)}s`;

  return (
    <Card className="border-chart-1/30 bg-chart-1/5">
      <CardContent className="space-y-5">
        <div className="flex flex-col items-center gap-3 pt-2">
          <div className="flex size-14 items-center justify-center border border-chart-1/30 bg-chart-1/10">
            <HugeiconsIcon
              icon={CheckmarkCircle02Icon}
              className="size-7 text-chart-1"
              strokeWidth={1.5}
            />
          </div>
          <div className="text-center">
            <h2 className="text-sm font-medium text-foreground">
              Subtitles Generated
            </h2>
            <p className="mt-1 text-xs text-muted-foreground">
              Processing complete
            </p>
          </div>
        </div>

        <Separator />

        <div className="space-y-2.5">
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">File</span>
            <span className="font-mono text-foreground">{fileName}</span>
          </div>
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">Segments</span>
            <span className="font-mono text-foreground">{segments}</span>
          </div>
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">Duration</span>
            <span className="font-mono text-foreground">
              {formatDuration(durationSecs)}
            </span>
          </div>
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">Location</span>
            <span className="max-w-[220px] truncate font-mono text-foreground">
              {dir}
            </span>
          </div>
          {transcriptionLog && (
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">Transcription Log</span>
              <span className="max-w-[220px] truncate font-mono text-foreground">
                {transcriptionLog.split(/[/\\]/).pop()}
              </span>
            </div>
          )}
        </div>

        <div className="flex gap-2">
          <Button
            variant="outline"
            size="lg"
            className="flex-1"
            onClick={openInExplorer}
          >
            <HugeiconsIcon icon={FolderOpenIcon} className="size-4" strokeWidth={1.5} />
            Open Folder
          </Button>
          <Button size="lg" className="flex-1" onClick={onReset}>
            <HugeiconsIcon icon={RefreshIcon} className="size-4" strokeWidth={1.5} />
            New File
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
