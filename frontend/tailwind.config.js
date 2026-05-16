/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    borderRadius: {
      none: '0',
      sm: '0.3rem',
      DEFAULT: '0.3rem',
      md: '0.3rem',
      lg: '0.3rem',
      xl: '0.3rem',
      '2xl': '0.3rem',
      '3xl': '0.3rem',
      '4xl': '0.3rem',
      full: '0.3rem'
    },
    extend: {
      colors: {
        gray: {
          50: '#f8f4ee',
          100: '#efe8dc',
          200: '#e0d4c4',
          300: '#cbb9a5',
          400: '#ad967d',
          500: '#89745f',
          600: '#6a5948',
          700: '#4e4135',
          800: '#332b24',
          900: '#1f1a15',
          950: '#14100d'
        },
        primary: {
          50: '#fff7ed',
          100: '#feebd3',
          200: '#fdd59f',
          300: '#fbb869',
          400: '#f7943c',
          500: '#ea7c2f',
          600: '#d86423',
          700: '#b64c1b',
          800: '#903b18',
          900: '#6e2e16',
          950: '#3d170d'
        },
        accent: {
          50: '#f7f8fb',
          100: '#e8edf5',
          200: '#cad4e3',
          300: '#a7b7ce',
          400: '#8292ae',
          500: '#627189',
          600: '#4d5b72',
          700: '#394353',
          800: '#252d39',
          900: '#151a22',
          950: '#0a0d12'
        },
        dark: {
          50: '#f7f8fb',
          100: '#eceff4',
          200: '#d7dee8',
          300: '#b4bfce',
          400: '#8b96a8',
          500: '#667385',
          600: '#4a5564',
          700: '#303846',
          800: '#1f2630',
          900: '#131822',
          950: '#0b0f14'
        }
      },
      fontFamily: {
        sans: [
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 18px 48px rgba(60, 45, 30, 0.11), 0 1px 0 rgba(255,255,255,0.68) inset',
        'glass-sm': '0 10px 26px rgba(60, 45, 30, 0.085), 0 1px 0 rgba(255,255,255,0.6) inset',
        glow: '0 0 20px rgba(234, 124, 47, 0.28)',
        'glow-lg': '0 0 40px rgba(234, 124, 47, 0.36)',
        card:
          '0 1px 2px rgba(60, 45, 30, 0.05), 0 12px 30px rgba(60, 45, 30, 0.075), inset 0 1px 0 rgba(255,255,255,0.68)',
        'card-hover': '0 18px 46px rgba(60, 45, 30, 0.14), inset 0 1px 0 rgba(255,255,255,0.78)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.06)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #f7943c 0%, #d86423 100%)',
        'gradient-dark': 'linear-gradient(135deg, #1f2630 0%, #0b0f14 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.12) 0%, rgba(255,255,255,0.06) 100%)',
        'mesh-gradient':
          'radial-gradient(at 14% 18%, rgba(234, 124, 47, 0.075) 0px, transparent 46%), radial-gradient(at 82% 8%, rgba(98, 113, 137, 0.06) 0px, transparent 44%), radial-gradient(at 52% 82%, rgba(251, 191, 36, 0.045) 0px, transparent 46%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.18s ease-out',
        'slide-up': 'slideUp 0.18s ease-out',
        'slide-down': 'slideDown 0.16s ease-out',
        'slide-in-right': 'slideInRight 0.18s ease-out',
        'scale-in': 'scaleIn 0.16s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 0 20px rgba(234, 124, 47, 0.24)' },
          '100%': { boxShadow: '0 0 30px rgba(234, 124, 47, 0.38)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      }
    }
  },
  plugins: []
}
