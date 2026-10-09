document.addEventListener('DOMContentLoaded', function () {
  const carousels = document.querySelectorAll('.carousel-widget');

  carousels.forEach(function (carousel) {
    const track = carousel.querySelector('.carousel-widget__track');
    const slides = carousel.querySelectorAll('.carousel-widget__slide');
    const prevBtn = carousel.querySelector('.carousel-widget__arrow--prev');
    const nextBtn = carousel.querySelector('.carousel-widget__arrow--next');
    const dots = carousel.querySelectorAll('.carousel-widget__dot');

    if (slides.length <= 1) return;

    let currentIndex = 0;
    let timer = null;
    const intervalMs = parseInt(carousel.getAttribute('data-autoplay') || '5000', 10);

    function goToSlide(index) {
      if (index < 0) {
        currentIndex = slides.length - 1;
      } else if (index >= slides.length) {
        currentIndex = 0;
      } else {
        currentIndex = index;
      }

      slides.forEach(function (slide, idx) {
        if (idx === currentIndex) {
          slide.classList.add('is-active');
        } else {
          slide.classList.remove('is-active');
        }
      });

      dots.forEach(function (dot, idx) {
        if (idx === currentIndex) {
          dot.classList.add('is-active');
          dot.setAttribute('aria-selected', 'true');
        } else {
          dot.classList.remove('is-active');
          dot.setAttribute('aria-selected', 'false');
        }
      });

      if (track) {
        track.style.transform = 'translateX(-' + (currentIndex * 100) + '%)';
      }
    }

    function nextSlide() {
      goToSlide(currentIndex + 1);
    }

    function prevSlide() {
      goToSlide(currentIndex - 1);
    }

    function startAutoplay() {
      if (intervalMs > 0 && !timer) {
        timer = setInterval(nextSlide, intervalMs);
      }
    }

    function stopAutoplay() {
      if (timer) {
        clearInterval(timer);
        timer = null;
      }
    }

    if (nextBtn) {
      nextBtn.addEventListener('click', function () {
        nextSlide();
        stopAutoplay();
        startAutoplay();
      });
    }

    if (prevBtn) {
      prevBtn.addEventListener('click', function () {
        prevSlide();
        stopAutoplay();
        startAutoplay();
      });
    }

    dots.forEach(function (dot, idx) {
      dot.addEventListener('click', function () {
        goToSlide(idx);
        stopAutoplay();
        startAutoplay();
      });
    });

    // Touch swipe support
    let touchStartX = 0;
    let touchEndX = 0;
    const swipeThreshold = 40;

    carousel.addEventListener('touchstart', function (e) {
      if (e.touches && e.touches.length > 0) {
        touchStartX = e.touches[0].clientX;
      }
    }, { passive: true });

    carousel.addEventListener('touchend', function (e) {
      if (e.changedTouches && e.changedTouches.length > 0) {
        touchEndX = e.changedTouches[0].clientX;
        handleSwipe();
      }
    }, { passive: true });

    function handleSwipe() {
      const diff = touchStartX - touchEndX;
      if (Math.abs(diff) >= swipeThreshold) {
        if (diff > 0) {
          nextSlide();
        } else {
          prevSlide();
        }
        stopAutoplay();
        startAutoplay();
      }
    }

    // Pause on hover / focus
    carousel.addEventListener('mouseenter', stopAutoplay);
    carousel.addEventListener('mouseleave', startAutoplay);
    carousel.addEventListener('focusin', stopAutoplay);
    carousel.addEventListener('focusout', startAutoplay);

    // Initial state
    goToSlide(0);
    startAutoplay();
  });
});

window.copySlidePromo = function (btn, code) {
  if (!code) return;
  const originalText = btn.textContent;

  function setCopied() {
    btn.textContent = 'copied!';
    btn.classList.add('is-copied');
    setTimeout(function () {
      btn.textContent = originalText;
      btn.classList.remove('is-copied');
    }, 2000);
  }

  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(code).then(setCopied).catch(function () {
      fallbackCopy(code, setCopied);
    });
  } else {
    fallbackCopy(code, setCopied);
  }
};

function fallbackCopy(text, callback) {
  const input = document.createElement('input');
  input.value = text;
  input.style.position = 'fixed';
  input.style.opacity = '0';
  document.body.appendChild(input);
  input.focus();
  input.select();
  try {
    document.execCommand('copy');
    callback();
  } catch (err) {
    // Ignore copy error
  }
  document.body.removeChild(input);
}
