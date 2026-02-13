/** @type {import('tailwindcss').Config} */
export default {
  darkMode: ["class"],
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
  	extend: {
  		fontFamily: {
  			sans: [
  				'Inter',
  				'system-ui',
  				'-apple-system',
  				'BlinkMacSystemFont',
  				'Segoe UI',
  				'Roboto',
  				'Helvetica Neue',
  				'Arial',
  				'sans-serif'
  			],
  			mono: [
  				'SF Mono',
  				'Monaco',
  				'Inconsolata',
  				'Roboto Mono',
  				'monospace'
  			]
  		},
  		fontSize: {
  			xs: [
  				'var(--font-size-xs)',
  				{
  					lineHeight: 'var(--line-height-normal)'
  				}
  			],
  			sm: [
  				'var(--font-size-sm)',
  				{
  					lineHeight: 'var(--line-height-normal)'
  				}
  			],
  			base: [
  				'var(--font-size-base)',
  				{
  					lineHeight: 'var(--line-height-normal)'
  				}
  			],
  			lg: [
  				'var(--font-size-lg)',
  				{
  					lineHeight: 'var(--line-height-relaxed)'
  				}
  			],
  			xl: [
  				'var(--font-size-xl)',
  				{
  					lineHeight: 'var(--line-height-snug)'
  				}
  			],
  			'2xl': [
  				'var(--font-size-2xl)',
  				{
  					lineHeight: 'var(--line-height-snug)'
  				}
  			],
  			'3xl': [
  				'var(--font-size-3xl)',
  				{
  					lineHeight: 'var(--line-height-tight)'
  				}
  			],
  			'4xl': [
  				'var(--font-size-4xl)',
  				{
  					lineHeight: 'var(--line-height-tight)'
  				}
  			],
  			'primary-foundation': [
  				'var(--font-size-primary)',
  				{
  					lineHeight: 'var(--line-height-foundation)',
  					fontWeight: 'var(--font-weight-primary)'
  				}
  			],
  			'secondary-foundation': [
  				'var(--font-size-secondary)',
  				{
  					lineHeight: 'var(--line-height-foundation)',
  					fontWeight: 'var(--font-weight-secondary)'
  				}
  			],
  			'tertiary-foundation': [
  				'var(--font-size-tertiary)',
  				{
  					lineHeight: 'var(--line-height-foundation)',
  					fontWeight: 'var(--font-weight-tertiary)'
  				}
  			],
  			'mono-foundation': [
  				'var(--font-size-mono)',
  				{
  					lineHeight: 'var(--line-height-foundation)',
  					fontFamily: 'var(--font-mono)'
  				}
  			]
  		},
  		fontWeight: {
  			light: 'var(--font-weight-light)',
  			normal: 'var(--font-weight-normal)',
  			medium: 'var(--font-weight-medium)',
  			semibold: 'var(--font-weight-semibold)',
  			bold: 'var(--font-weight-bold)'
  		},
  		lineHeight: {
  			tight: 'var(--line-height-tight)',
  			snug: 'var(--line-height-snug)',
  			normal: 'var(--line-height-normal)',
  			relaxed: 'var(--line-height-relaxed)',
  			loose: 'var(--line-height-loose)'
  		},
  		spacing: {
  			'0': 'var(--space-0)',
  			'1': 'var(--space-1)',
  			'2': 'var(--space-2)',
  			'3': 'var(--space-3)',
  			'4': 'var(--space-4)',
  			'5': 'var(--space-5)',
  			'6': 'var(--space-6)',
  			'8': 'var(--space-8)',
  			'10': 'var(--space-10)',
  			'12': 'var(--space-12)',
  			'16': 'var(--space-16)',
  			'20': 'var(--space-20)',
  			'24': 'var(--space-24)'
  		},
  		borderRadius: {
  			xs: 'var(--radius-xs)',
  			sm: 'var(--radius-sm)',
  			md: 'var(--radius-md)',
  			lg: 'var(--radius-lg)',
  			xl: 'var(--radius-xl)',
  			'2xl': 'var(--radius-2xl)'
  		},
  		boxShadow: {
  			xs: 'var(--shadow-xs)',
  			sm: 'var(--shadow-sm)',
  			md: 'var(--shadow-md)',
  			lg: 'var(--shadow-lg)',
  			xl: 'var(--shadow-xl)',
  			'2xl': 'var(--shadow-2xl)'
  		},
  		transitionDuration: {
  			fast: 'var(--transition-fast)',
  			base: 'var(--transition-base)',
  			slow: 'var(--transition-slow)',
  			bounce: 'var(--transition-bounce)'
  		},
  		transitionTimingFunction: {
  			smooth: 'cubic-bezier(0.23, 1, 0.32, 1)',
  			bounce: 'cubic-bezier(0.34, 1.56, 0.64, 1)'
  		},
  		colors: {
  			background: 'var(--background)',
  			foreground: 'var(--foreground)',
  			'bg-primary': 'var(--bg-primary)',
  			'bg-secondary': 'var(--bg-secondary)',
  			'bg-tertiary': 'var(--bg-tertiary)',
  			'bg-elevated': 'var(--bg-elevated)',
  			'bg-overlay': 'var(--bg-overlay)',
  			'bg-hover': 'var(--bg-hover)',
  			'bg-active': 'var(--bg-active)',
  			'text-primary': 'var(--text-primary)',
  			'text-secondary': 'var(--text-secondary)',
  			'text-tertiary': 'var(--text-tertiary)',
  			'text-quaternary': 'var(--text-quaternary)',
  			'text-disabled': 'var(--text-disabled)',
  			'text-inverse': 'var(--text-inverse)',
  			primary: {
  				DEFAULT: 'var(--primary)',
  				foreground: 'var(--primary-foreground)',
  				hover: 'var(--primary-hover)'
  			},
  			secondary: {
  				DEFAULT: 'var(--secondary)',
  				foreground: 'var(--secondary-foreground)',
  				hover: 'var(--secondary-hover)'
  		},
  		'accent-primary': 'var(--accent-primary)',
  		'accent-primary-hover': 'var(--accent-primary-hover)',
  		'accent-secondary': 'var(--accent-secondary)',
  			card: {
  				DEFAULT: 'var(--card)',
  				foreground: 'var(--card-foreground)',
  				border: 'var(--card-border)',
  				hover: 'var(--card-hover)'
  			},
  			popover: {
  				DEFAULT: 'var(--popover)',
  				foreground: 'var(--popover-foreground)'
  			},
  			muted: {
  				DEFAULT: 'var(--muted)',
  				foreground: 'var(--muted-foreground)'
  			},
  			accent: {
  				DEFAULT: 'var(--accent)',
  				foreground: 'var(--accent-foreground)'
  			},
  			destructive: {
  				DEFAULT: 'var(--destructive)',
  				foreground: 'var(--destructive-foreground)'
  			},
  			'status-success': {
  				DEFAULT: 'var(--status-success)',
  				light: 'var(--status-success-light)',
  				bg: 'var(--status-success-bg)',
  				border: 'var(--status-success-border)'
  			},
  			'status-warning': {
  				DEFAULT: 'var(--status-warning)',
  				light: 'var(--status-warning-light)',
  				bg: 'var(--status-warning-bg)',
  				border: 'var(--status-warning-border)'
  			},
  			'status-error': {
  				DEFAULT: 'var(--status-error)',
  				light: 'var(--status-error-light)',
  				bg: 'var(--status-error-bg)',
  				border: 'var(--status-error-border)'
  			},
  			'status-info': {
  				DEFAULT: 'var(--status-info)',
  				light: 'var(--status-info-light)',
  				bg: 'var(--status-info-bg)',
  				border: 'var(--status-info-border)'
  			},
  			'status-neutral': {
  				DEFAULT: 'var(--status-neutral)',
  				light: 'var(--status-neutral-light)',
  				bg: 'var(--status-neutral-bg)',
  				border: 'var(--status-neutral-border)'
  			},
  			border: {
  				DEFAULT: 'var(--border)',
  				primary: 'var(--border)',
  				secondary: 'var(--border-secondary)',
  				tertiary: 'var(--border-tertiary)'
  			},
  			'border-primary': 'var(--border)',
  			'border-secondary': 'var(--border-secondary)',
  			'border-tertiary': 'var(--border-tertiary)',
  			input: {
  				DEFAULT: 'var(--input)',
  				focus: 'var(--input-focus)'
  			},
  			ring: {
  				DEFAULT: 'var(--ring)',
  				offset: 'var(--ring-offset)'
  			},
  			nav: {
  				background: 'var(--nav-background)',
  				elevated: 'var(--nav-elevated)',
  				border: 'var(--nav-border)',
  				'text-primary': 'var(--nav-text-primary)',
  				'text-secondary': 'var(--nav-text-secondary)',
  				'text-tertiary': 'var(--nav-text-tertiary)',
  				'text-active': 'var(--nav-text-active)',
  				'text-hover': 'var(--nav-text-hover)',
  				'active-bg': 'var(--nav-active-bg)',
  				'hover-bg': 'var(--nav-hover-bg)',
  				'focus-ring': 'var(--nav-focus-ring)'
  			},
  			chart: {
  				'1': 'var(--chart-1)',
  				'2': 'var(--chart-2)',
  				'3': 'var(--chart-3)',
  				'4': 'var(--chart-4)',
  				'5': 'var(--chart-5)',
  				'6': 'var(--chart-6)',
  				'7': 'var(--chart-7)',
  				'8': 'var(--chart-8)'
  			},
  			sidebar: {
  				DEFAULT: 'var(--sidebar-background)',
  				foreground: 'var(--sidebar-foreground)',
  				primary: 'var(--sidebar-primary)',
  				'primary-foreground': 'var(--sidebar-primary-foreground)',
  				accent: 'var(--sidebar-accent)',
  				'accent-foreground': 'var(--sidebar-accent-foreground)',
  				border: 'var(--sidebar-border)',
  				ring: 'var(--sidebar-ring)'
  			}
  		},
  		animation: {
  			'fade-in': 'fadeIn var(--transition-base) ease-out',
  			'slide-in': 'slideIn var(--transition-base) ease-out',
  			'bounce-in': 'bounceIn var(--transition-bounce) ease-out'
  		},
  		keyframes: {
  			fadeIn: {
  				'0%': {
  					opacity: '0',
  					transform: 'translateY(4px)'
  				},
  				'100%': {
  					opacity: '1',
  					transform: 'translateY(0)'
  				}
  			},
  			slideIn: {
  				'0%': {
  					opacity: '0',
  					transform: 'translateX(-8px)'
  				},
  				'100%': {
  					opacity: '1',
  					transform: 'translateX(0)'
  				}
  			},
  			bounceIn: {
  				'0%': {
  					opacity: '0',
  					transform: 'scale(0.95)'
  				},
  				'50%': {
  					opacity: '0.8',
  					transform: 'scale(1.02)'
  				},
  				'100%': {
  					opacity: '1',
  					transform: 'scale(1)'
  				}
  			}
  		},
  		backdropBlur: {
  			xs: '2px',
  			sm: '4px',
  			md: '8px',
  			lg: '12px',
  			xl: '16px',
  			'2xl': '24px',
  			'3xl': '40px'
  		}
  	}
  },
  plugins: [
    require("tailwindcss-animate"),
    // Custom plugin for Linear-inspired utilities
    function({ addUtilities, theme }) {
      const newUtilities = {
        // Typography utilities
        '.text-display': {
          fontSize: 'var(--font-size-4xl)',
          lineHeight: 'var(--line-height-tight)',
          fontWeight: 'var(--font-weight-bold)',
          letterSpacing: '-0.025em',
          color: 'var(--text-primary)',
        },
        '.text-heading-1': {
          fontSize: 'var(--font-size-3xl)',
          lineHeight: 'var(--line-height-tight)',
          fontWeight: 'var(--font-weight-semibold)',
          letterSpacing: '-0.025em',
          color: 'var(--text-primary)',
        },
        '.text-heading-2': {
          fontSize: 'var(--font-size-2xl)',
          lineHeight: 'var(--line-height-snug)',
          fontWeight: 'var(--font-weight-semibold)',
          letterSpacing: '-0.025em',
          color: 'var(--text-primary)',
        },
        '.text-heading-3': {
          fontSize: 'var(--font-size-lg)',
          lineHeight: 'var(--line-height-snug)',
          fontWeight: 'var(--font-weight-medium)',
          color: 'var(--text-primary)',
        },
        '.text-body-large': {
          fontSize: 'var(--font-size-lg)',
          lineHeight: 'var(--line-height-relaxed)',
          color: 'var(--text-secondary)',
        },
        '.text-body': {
          fontSize: 'var(--font-size-base)',
          lineHeight: 'var(--line-height-normal)',
          color: 'var(--text-secondary)',
        },
        '.text-body-small': {
          fontSize: 'var(--font-size-sm)',
          lineHeight: 'var(--line-height-normal)',
          color: 'var(--text-tertiary)',
        },
        '.text-caption': {
          fontSize: 'var(--font-size-xs)',
          lineHeight: 'var(--line-height-normal)',
          color: 'var(--text-quaternary)',
          fontWeight: 'var(--font-weight-medium)',
          textTransform: 'uppercase',
          letterSpacing: '0.05em',
        },

        // Interactive utilities
        '.interactive-hover': {
          transition: 'all var(--transition-fast)',
          cursor: 'pointer',
          '&:hover': {
            backgroundColor: 'var(--bg-hover)',
          },
          '&:active': {
            backgroundColor: 'var(--bg-active)',
            transform: 'translateY(0.5px)',
          },
        },

        // Focus utilities
        '.focus-ring': {
          outline: '2px solid transparent',
          outlineOffset: '2px',
          '&:focus-visible': {
            outline: '2px solid var(--ring)',
            outlineOffset: '2px',
          },
        },

        // Card utilities
        '.card-elevated': {
          backgroundColor: 'var(--card)',
          border: '1px solid var(--card-border)',
          borderRadius: 'var(--radius-lg)',
          boxShadow: 'var(--shadow-sm)',
          transition: 'all var(--transition-base)',
          '&:hover': {
            backgroundColor: 'var(--card-hover)',
            borderColor: 'var(--border)',
            boxShadow: 'var(--shadow-md)',
          },
        },

        // Status utilities
        '.status-success': {
          color: 'var(--status-success-light)',
          backgroundColor: 'var(--status-success-bg)',
          borderColor: 'var(--status-success-border)',
        },
        '.status-warning': {
          color: 'var(--status-warning-light)',
          backgroundColor: 'var(--status-warning-bg)',
          borderColor: 'var(--status-warning-border)',
        },
        '.status-error': {
          color: 'var(--status-error-light)',
          backgroundColor: 'var(--status-error-bg)',
          borderColor: 'var(--status-error-border)',
        },
        '.status-info': {
          color: 'var(--status-info-light)',
          backgroundColor: 'var(--status-info-bg)',
          borderColor: 'var(--status-info-border)',
        },
        '.status-neutral': {
          color: 'var(--status-neutral-light)',
          backgroundColor: 'var(--status-neutral-bg)',
          borderColor: 'var(--status-neutral-border)',
        },

        // Gradient utilities
        '.gradient-accent': {
          background: 'var(--accent-gradient)',
        },
        '.gradient-text': {
          background: 'var(--accent-gradient)',
          WebkitBackgroundClip: 'text',
          WebkitTextFillColor: 'transparent',
          backgroundClip: 'text',
        },

        // Glass morphism utilities
        '.glass': {
          backgroundColor: 'color-mix(in oklch, var(--bg-overlay) 80%, transparent)',
          backdropFilter: 'blur(12px)',
          WebkitBackdropFilter: 'blur(12px)',
          border: '1px solid var(--border-secondary)',
        },

        // Scrollbar utilities
        '.scrollbar-thin': {
          '&::-webkit-scrollbar': {
            width: '6px',
            height: '6px',
          },
          '&::-webkit-scrollbar-track': {
            background: 'transparent',
          },
          '&::-webkit-scrollbar-thumb': {
            background: 'var(--border)',
            borderRadius: '3px',
          },
          '&::-webkit-scrollbar-thumb:hover': {
            background: 'var(--text-quaternary)',
          },
        },
        '.scrollbar-track-transparent': {
          '&::-webkit-scrollbar-track': {
            background: 'transparent',
          },
        },
        '.scrollbar-thumb-border': {
          '&::-webkit-scrollbar-thumb': {
            background: 'var(--border)',
            borderRadius: '3px',
          },
          '&::-webkit-scrollbar-thumb:hover': {
            background: 'var(--text-quaternary)',
          },
        },
        '.scrollbar-none': {
          '&::-webkit-scrollbar': {
            display: 'none',
            width: '0px',
            background: 'transparent',
          },
          '-ms-overflow-style': 'none',
          'scrollbar-width': 'none',
        },
        '.scrollbar-auto': {
          '&::-webkit-scrollbar': {
            width: '12px',
            height: '12px',
          },
          '&::-webkit-scrollbar-track': {
            background: 'var(--muted)',
          },
          '&::-webkit-scrollbar-thumb': {
            background: 'var(--border)',
            borderRadius: '6px',
          },
          '&::-webkit-scrollbar-thumb:hover': {
            background: 'var(--text-quaternary)',
          },
        },

        // Text selection utilities
        '.select-none': {
          WebkitUserSelect: 'none',
          MozUserSelect: 'none',
          msUserSelect: 'none',
          userSelect: 'none',
        },
        '.select-text': {
          WebkitUserSelect: 'text',
          MozUserSelect: 'text',
          msUserSelect: 'text',
          userSelect: 'text',
        },
        '.select-all': {
          WebkitUserSelect: 'all',
          MozUserSelect: 'all',
          msUserSelect: 'all',
          userSelect: 'all',
        },
        '.select-auto': {
          WebkitUserSelect: 'auto',
          MozUserSelect: 'auto',
          msUserSelect: 'auto',
          userSelect: 'auto',
        },
      }

      addUtilities(newUtilities)
    }
  ],
}
