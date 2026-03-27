import { Button } from "@/components/ui/button";
import { HugeiconsIcon } from "@hugeicons/react";
import { Alert02Icon } from "@hugeicons/core-free-icons";
import { formatSetupIssue } from "@/lib/setupHelpers";
import type { SetupStatusResponse } from "@/lib/types";

interface SetupBannerProps {
  setupStatus: SetupStatusResponse;
  onInstall: (actionId: string) => void;
  disabled?: boolean;
}

export function SetupBanner({ setupStatus, onInstall, disabled }: SetupBannerProps) {
  const servicesWithIssues = setupStatus.services.filter(
    (s) => s.state === "action_required"
  );

  if (servicesWithIssues.length === 0) return null;

  return (
    <div className="space-y-3">
      {servicesWithIssues.map((service) => {
        const issues = service.issues ?? [];
        const actions = service.actions ?? [];
        const actionableActions = actions.filter((action) => action.kind !== "manual");
        const archiveActions = actions.filter((action) => action.kind === "archive");
        const manualActions = actions.filter((action) => action.kind === "manual");

        return (
          <div
            key={service.id}
            className="border border-chart-4/30 bg-chart-4/5 p-4 space-y-3"
          >
            <div className="flex items-start gap-3">
              <HugeiconsIcon
                icon={Alert02Icon}
                className="mt-0.5 size-4 shrink-0 text-chart-4"
                strokeWidth={2}
              />
              <div className="flex-1 space-y-1">
                <p className="text-xs font-medium text-foreground">
                  {service.display_name}{" "}
                  <span className="text-muted-foreground font-normal">
                    — required for {service.required_for}
                  </span>
                </p>
                {issues.map((issue, i) => (
                  <p key={i} className="text-[11px] text-muted-foreground">
                    {formatSetupIssue(issue)}
                  </p>
                ))}
              </div>
            </div>

            {actionableActions.length > 0 && (
              <div className="flex flex-wrap gap-2 pl-7">
                {actionableActions.map((action) => (
                    <Button
                      key={action.id}
                      size="sm"
                      variant={action.preferred ? "default" : "outline"}
                      className="text-[11px]"
                      onClick={() => onInstall(action.id)}
                      disabled={disabled}
                    >
                      {action.label}
                    </Button>
                  ))}
              </div>
            )}

            {manualActions.length > 0 && (
              <div className="space-y-2 pl-7">
                {manualActions.map((action) => (
                  <div
                    key={action.id}
                    className="space-y-1 border border-border/60 bg-background/50 p-3"
                  >
                    <p className="text-[11px] font-medium text-foreground">
                      {action.label}
                    </p>
                    <p className="text-[10px] text-muted-foreground">
                      {action.description}
                    </p>
                    {action.guidance && (
                      <p className="text-[10px] text-muted-foreground">
                        {action.guidance}
                      </p>
                    )}
                  </div>
                ))}
              </div>
            )}

            {archiveActions.length > 0 && (
              <div className="pl-7 space-y-0.5">
                {archiveActions.map((action) => (
                  <p key={action.id} className="text-[10px] text-muted-foreground">
                    {action.preferred ? "▸ " : "  "}
                    {action.description}
                  </p>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
