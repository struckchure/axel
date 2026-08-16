import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../../lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-full text-sm font-medium transition-all duration-200 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0 cursor-pointer",
  {
    variants: {
      variant: {
        default:
          "bg-orange-600 text-white shadow-md shadow-orange-600/30 hover:bg-orange-500 hover:shadow-lg hover:shadow-orange-600/40 hover:-translate-y-0.5 active:translate-y-0",
        secondary:
          "border border-zinc-200 bg-white text-zinc-900 shadow-sm hover:border-orange-500/50 hover:bg-zinc-100 dark:border-[#232738] dark:bg-[#12141c] dark:text-slate-200 dark:hover:bg-[#181b26] dark:hover:text-white hover:-translate-y-0.5",
        outline:
          "border border-zinc-200 bg-transparent text-zinc-700 hover:bg-zinc-100 hover:text-zinc-900 dark:border-[#232738] dark:text-slate-300 dark:hover:bg-slate-800/50 dark:hover:text-white",
        ghost:
          "text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 dark:text-slate-400 dark:hover:bg-slate-800/40 dark:hover:text-slate-100",
        link:
          "text-orange-600 dark:text-orange-400 underline-offset-4 hover:underline",
      },
      size: {
        default: "h-9 px-5 py-2",
        sm: "h-8 rounded-full px-3 text-xs",
        lg: "h-11 rounded-full px-7 text-base font-semibold",
        icon: "h-9 w-9",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => {
    return (
      <button
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    );
  }
);
Button.displayName = "Button";

export { Button, buttonVariants };
