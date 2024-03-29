from conans import ConanFile, tools
import os

source_dir = os.getenv("FW_SRC_DIR", None)


class ParticlePackage(ConanFile):
    name = "ppc"
    version = os.getenv("FW_VERSION", "snapshot")
    settings = "os", "compiler", "build_type", "arch"
    url = "https://github.com/jw3/ppc"
    generators = "cmake"
    requires = ['particle-common/snapshot@jw3/stable', 'MQTT/snapshot@jw3/stable']

    def package(self):
        self.copy("*.cmake", src=f"{source_dir}/cmake")
        self.copy("*.c*", dst="src", src=f"{source_dir}/src")
        self.copy("*.h*", dst="include", src=f"{source_dir}/src")
