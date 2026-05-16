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
          50: '#fff7f1',
          100: '#f9e3d3',
          200: '#f3c8aa',
          300: '#e9aa82',
          400: '#dd8656',
          500: '#cf7444',
          600: '#bf6334',
          700: '#9a4c28',
          800: '#743a23',
          900: '#552c1d',
          950: '#30150d'
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
          50: '#f4eee6',
          100: '#e5ddd3',
          200: '#cfc5b9',
          300: '#a79f95',
          400: '#81796f',
          500: '#67707a',
          600: '#46515c',
          700: '#2f3740',
          800: '#242a31',
          900: '#181c21',
          950: '#0e1114'
        }
      },
      fontFamily: {
        sans: [
          'IBM Plex Sans',
          'PingFang SC',
          'Microsoft YaHei',
          'system-ui',
          'sans-serif'
        ],
        display: ['Space Grotesk', 'IBM Plex Sans', 'PingFang SC', 'Microsoft YaHei', 'sans-serif'],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 1px 2px rgba(41, 30, 20, 0.08), 0 14px 34px rgba(41, 30, 20, 0.08)',
        'glass-sm': '0 1px 2px rgba(41, 30, 20, 0.08), 0 8px 20px rgba(41, 30, 20, 0.06)',
        glow: '0 8px 20px rgba(221, 134, 86, 0.22)',
        'glow-lg': '0 16px 36px rgba(221, 134, 86, 0.26)',
        card: '0 1px 2px rgba(41, 30, 20, 0.08), 0 14px 34px rgba(41, 30, 20, 0.08)',
        'card-hover': '0 20px 56px rgba(41, 30, 20, 0.16)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.06)'
      },
      backgroundImage: {
        'gradient-radial': 'linear-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #dd8656 0%, #bf6334 100%)',
        'gradient-dark': 'linear-gradient(135deg, #181c21 0%, #0e1114 100%)',
        'gradient-glass': 'linear-gradient(135deg, rgba(255,253,248,0.96) 0%, rgba(247,240,231,0.92) 100%)',
        'mesh-gradient':
          'linear-gradient(rgba(23, 26, 31, 0.035) 1px, transparent 1px), linear-gradient(90deg, rgba(23, 26, 31, 0.035) 1px, transparent 1px)'
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
