import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../../lib/utils";

const badgeVariants = cva(
  "inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold tracking-wide transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2",
  {
    variants: {
      variant: {
        default:
          "border border-orange-500/30 bg-orange-500/10 text-orange-400 hover:bg-orange-500/20",
        secondary:
          "border border-[#232738] bg-[#12141c] text-slate-300 hover:bg-[#181b26]",
        outline:
          "border border-[#232738] text-slate-400",
        tag:
          "bg-orange-500/10 text-orange-400 font-bold uppercase tracking-wider text-[0.7rem] px-2.5 py-0.5 border border-orange-500/25",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <div className={cn(badgeVariants({ variant }), className)} {...props} />
  );
}

export { Badge, badgeVariants };
